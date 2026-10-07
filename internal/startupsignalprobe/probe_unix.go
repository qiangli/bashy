//go:build startup_signal_probe && (linux || darwin)

// Package startupsignalprobe supplies a test-only dependency-initialization
// barrier. It is never linked into ordinary or release builds.
package startupsignalprobe

import (
	"os"
	"syscall"
)

func init() {
	if os.Getenv("BASHY_TEST_STARTUP_STOP") != "1" {
		return
	}
	// The parent waits for this exact stop with waitpid(WUNTRACED), queues
	// the signal under test, and resumes us. Pending signals are delivered
	// before userspace resumes: main cannot race ahead and repair the ignore.
	if err := syscall.Kill(os.Getpid(), syscall.SIGSTOP); err != nil {
		panic(err)
	}
}
