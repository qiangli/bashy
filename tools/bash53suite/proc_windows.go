//go:build windows

package main

import (
	"os/exec"
	"strconv"
)

func configureProcess(cmd *exec.Cmd) {}

func killProcessTree(pid int) {
	_ = exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
