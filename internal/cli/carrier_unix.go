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
	"sync"
	"syscall"

	"mvdan.cc/sh/v3/interp"
)

// platformJobCarrier returns the OS-backed job carrier: one re-exec of this
// executable in helper mode per background job (see MaybeRunJobCarrierHelper),
// giving every `cmd &` a real, signalable kernel PID for `$!`.
func platformJobCarrier() interp.JobCarrier { return execJobCarrier{} }

type execJobCarrier struct{}

func (execJobCarrier) StartCarrier(ctx context.Context) (interp.CarrierProcess, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolving executable: %v", err)
	}
	cmd := exec.Command(exe, carrierHelperArg)
	// An empty environment: the helper is intercepted before flag parsing,
	// startup files and telemetry, and an env var that steers a normal shell
	// start (BASH_ENV, BASH_SETPGRP, OTEL_*) must not reach it anyway.
	cmd.Env = []string{}
	// Its own process group: under job control (`set -m`) group-directed
	// signals target the negated carrier PID, and a tty ^C aimed at the
	// shell's foreground group must not strike carriers.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, err
	}
	return &execCarrierProc{cmd: cmd, stdin: stdin}, nil
}

type execCarrierProc struct {
	cmd   *exec.Cmd
	stdin io.Closer
	term  sync.Once
}

func (p *execCarrierProc) Pid() int { return p.cmd.Process.Pid }

// Wait reaps the helper and maps a signal death to its signal number, 0 for a
// normal exit. The runner calls it exactly once.
func (p *execCarrierProc) Wait() int {
	_ = p.cmd.Wait() // a signal death surfaces as *exec.ExitError; ProcessState has the detail
	if ws, ok := p.cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return int(ws.Signal())
	}
	return 0
}

// Terminate makes the helper exit promptly: EOF on its stdin pipe is its exit
// condition, and the kill spares waiting for it to notice. Idempotent via the
// sync.Once, and safe to race with Wait — os.Process keeps Kill after reap a
// harmless ErrProcessDone.
func (p *execCarrierProc) Terminate() {
	p.term.Do(func() {
		p.stdin.Close()
		_ = p.cmd.Process.Kill()
	})
}
