// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"mvdan.cc/sh/v3/interp"
)

// platformJobCarrier returns the OS-backed job carrier: one re-exec of this
// executable in helper mode per background job (see MaybeRunJobCarrierHelper),
// giving every `cmd &` a real, signalable kernel PID for `$!`.
func platformJobCarrier() interp.JobCarrier { return execJobCarrier{} }

func resetJobCarrierSignals() {
	// The Go runtime installs handlers at startup for notify-only signals such
	// as SIGUSR1 and silently consumes them when no Notify receiver exists.
	// signal.Reset cannot restore those runtime-owned actions. Relay the common
	// terminating signals through a reserved 128+signal exit code instead;
	// execCarrierProc.Wait maps that code back to the signal the interpreter
	// must observe. This also overrides SIG_IGN inherited across exec.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch,
		syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGILL,
		syscall.SIGTRAP, syscall.SIGABRT, syscall.SIGBUS, syscall.SIGFPE,
		syscall.SIGUSR1, syscall.SIGSEGV, syscall.SIGUSR2, syscall.SIGPIPE,
		syscall.SIGALRM, syscall.SIGTERM, syscall.SIGTSTP, syscall.SIGTTIN,
		syscall.SIGTTOU, syscall.SIGXCPU, syscall.SIGXFSZ,
	)
	go func() {
		for sig := range ch {
			num, ok := sig.(syscall.Signal)
			if !ok {
				os.Exit(255)
			}
			switch num {
			case syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU:
				// Preserve job-control stop/resume visibility. SIGSTOP cannot
				// be caught; a later SIGCONT resumes this relay loop.
				_ = syscall.Kill(os.Getpid(), syscall.SIGSTOP)
			default:
				os.Exit(128 + int(num))
			}
		}
	}()
}

func signalJobCarrierReady() {
	if os.Getenv(carrierReadyEnv) != "3" {
		return
	}
	ready := os.NewFile(3, "bashy-job-carrier-ready")
	if ready == nil {
		return
	}
	_, _ = ready.Write([]byte{1})
	_ = ready.Close()
}

type execJobCarrier struct{}

func (execJobCarrier) StartCarrier(ctx context.Context) (interp.CarrierProcess, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolving executable: %v", err)
	}
	cmd := exec.Command(exe, carrierHelperArg)
	// Only the private readiness descriptor marker is inherited. The helper is
	// intercepted before flag parsing, startup files and telemetry; user-facing
	// environment such as BASH_ENV, BASH_SETPGRP and OTEL_* must not reach it.
	cmd.Env = []string{carrierReadyEnv + "=3"}
	// Its own process group: under job control (`set -m`) group-directed
	// signals target the negated carrier PID, and a tty ^C aimed at the
	// shell's foreground group must not strike carriers.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	readyR, readyW, err := os.Pipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	cmd.ExtraFiles = []*os.File{readyW}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		readyR.Close()
		readyW.Close()
		return nil, err
	}
	readyW.Close()
	_ = readyR.SetReadDeadline(time.Now().Add(5 * time.Second))
	var ready [1]byte
	_, readyErr := io.ReadFull(readyR, ready[:])
	readyR.Close()
	if readyErr != nil {
		stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("job carrier readiness: %v", readyErr)
	}
	return &execCarrierProc{cmd: cmd, stdin: stdin}, nil
}

type execCarrierProc struct {
	cmd   *exec.Cmd
	stdin io.Closer
	term  sync.Once
}

func (p *execCarrierProc) Pid() int { return p.cmd.Process.Pid }

// WaitState reaps or observes child stop state via wait4/waitpid with WUNTRACED.
func (p *execCarrierProc) WaitState() interp.CarrierWaitState {
	pid := p.cmd.Process.Pid
	var ws syscall.WaitStatus
	for {
		_, err := syscall.Wait4(pid, &ws, syscall.WUNTRACED, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			if p.cmd.ProcessState != nil {
				if ws, ok := p.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
					return interp.CarrierWaitState{Signal: int(ws.Signal()), Stopped: false}
				}
				if code := p.cmd.ProcessState.ExitCode(); code > 128 && code < 256 {
					return interp.CarrierWaitState{Signal: code - 128, Stopped: false}
				}
			}
			return interp.CarrierWaitState{Signal: 0, Stopped: false}
		}
		break
	}

	if sig, stopped := isWaitStatusStopped(ws); stopped {
		return interp.CarrierWaitState{
			Signal:  sig,
			Stopped: true,
		}
	}

	if ws.Signaled() {
		return interp.CarrierWaitState{Signal: int(ws.Signal()), Stopped: false}
	}
	if code := ws.ExitStatus(); code > 128 && code < 256 {
		return interp.CarrierWaitState{Signal: code - 128, Stopped: false}
	}
	return interp.CarrierWaitState{Signal: 0, Stopped: false}
}

// Wait reaps the helper and maps a signal death to its signal number, 0 for a
// normal exit. The runner calls it exactly once.
func (p *execCarrierProc) Wait() int {
	for {
		st := p.WaitState()
		if !st.Stopped {
			return st.Signal
		}
	}
}

// Terminate makes the helper exit promptly: EOF on its stdin pipe is its exit
// condition, and the kill spares waiting for it to notice. Idempotent via the
// sync.Once, and safe to race with Wait — os.Process keeps Kill after reap a
// harmless ErrProcessDone.
func (p *execCarrierProc) Terminate() {
	p.term.Do(func() {
		p.stdin.Close()
		_ = p.cmd.Process.Kill()
		_ = syscall.Kill(p.cmd.Process.Pid, syscall.SIGCONT)
	})
}

func isWaitStatusStopped(ws syscall.WaitStatus) (int, bool) {
	if ws.Stopped() {
		return int(ws.StopSignal()), true
	}
	u := uint32(ws)
	if u&0xff == 0x7f {
		sig := int((u >> 8) & 0xff)
		if sig > 0 && sig != 0x7f {
			return sig, true
		}
	}
	return 0, false
}
