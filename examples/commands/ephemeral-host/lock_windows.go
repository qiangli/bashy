//go:build windows

package main

import (
	"fmt"
	"os"
	"time"
)

// fileLock on Windows is an exclusively created sentinel file. It is not
// released if the process dies; a lock older than the wait is taken over.
type fileLock struct{ path string }

func acquireLock(path string, wait time.Duration) (*fileLock, error) {
	deadline := time.Now().Add(wait)
	for {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_ = f.Close()
			return &fileLock{path: path}, nil
		}
		if info, serr := os.Stat(path); serr == nil && time.Since(info.ModTime()) > wait {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("ledger lock %s: %w", path, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (l *fileLock) Release() error { return os.Remove(l.path) }
