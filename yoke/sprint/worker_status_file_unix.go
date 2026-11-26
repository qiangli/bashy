//go:build unix

package main

import (
	"os"
	"syscall"
)

func openWorkerStatusFile(path string) (*os.File, error) {
	// An untrusted file can be replaced with a FIFO between the pre-open
	// check and opening it. Never block in open or follow a substituted link.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
}
