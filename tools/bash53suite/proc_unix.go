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
