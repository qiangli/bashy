//go:build !windows

package adapter

import (
	"os"
	"syscall"
)

func forwardTermSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
