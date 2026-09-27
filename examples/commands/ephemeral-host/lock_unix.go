//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// fileLock is an exclusive flock on a sentinel file, released by the kernel
// if the process dies.
type fileLock struct{ f *os.File }

func acquireLock(path string, wait time.Duration) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &fileLock{f: f}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("ledger lock %s: %w", path, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (l *fileLock) Release() error { return l.f.Close() }
