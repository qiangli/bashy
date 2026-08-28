//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

type parentDeathWatch struct{}

func configureProcess(cmd *exec.Cmd) {}

func killProcessTree(pid int) {
	_ = exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}

func armParentDeathWatch(_ *exec.Cmd) (*parentDeathWatch, error) {
	return nil, fmt.Errorf("parent-death guard: unsupported on windows")
}
func parentDeathWatchStarted(_ *parentDeathWatch, _ error) {}
func stopParentDeathWatch(_ *parentDeathWatch)             {}
