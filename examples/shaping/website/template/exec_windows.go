//go:build windows

package main

import (
	"os"
	"os/exec"
)

func replaceProcess(bin string, args, env []string) error {
	cmd := exec.Command(bin, args[1:]...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
