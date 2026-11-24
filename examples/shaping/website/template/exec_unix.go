//go:build !windows

package main

import "syscall"

func replaceProcess(bin string, args, env []string) error {
	return syscall.Exec(bin, args, env)
}
