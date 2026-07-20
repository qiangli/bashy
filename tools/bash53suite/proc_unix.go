//go:build unix

package main

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// Note on the container-runtime signal-mask leak (the `trap`/`varenv`/`execscript`
// SIGRTMIN failure seen ONLY under k8s/DKS): a CRI runtime sets the realtime
// signals to SIG_IGN on the pod's PID 1, SIG_IGN survives exec, so every bash
// fixture inherits it and `trap -p` lists a spurious `trap -- '' SIGRTMIN`. This
// cannot be fixed from inside this Go process: the Go runtime snapshots which
// signals were ignored at init and RE-IGNORES them in every exec'd child, so
// neither os/signal.Notify nor a raw sigaction here reaches the fixture. The
// disposition must be reset BEFORE the Go binary starts — see the `sigdfl` C
// shim baked into tools/bash53-container/Containerfile.k8s, which the pod runs
// ahead of this harness.

func killProcessTree(pid int) {
	for _, child := range childPIDs(pid) {
		killProcessTree(child)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func childPIDs(pid int) []int {
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(bytes.TrimSpace(out)))
	pids := make([]int, 0, len(fields))
	for _, f := range fields {
		if child, err := strconv.Atoi(f); err == nil && child > 1 {
			pids = append(pids, child)
		}
	}
	return pids
}
