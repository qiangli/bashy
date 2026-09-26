// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines && !windows

package agentos

// Sprint: #290; Story: #951; Story-ID: 7b6008a8e333

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// A termination signal sent to bashy while it fronts an engine
// (`bashy ollama serve`) reaches the engine: the passthrough returns once the
// engine exits instead of bashy dying and orphaning it.
func TestEnginePassthroughForwardsTermination(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep unavailable")
	}
	done := make(chan int, 1)
	start := time.Now()
	go func() { done <- execEnginePassthrough(sleep, []string{"30"}) }()
	time.Sleep(300 * time.Millisecond) // let the child start and Notify register
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code == 0 {
			t.Fatalf("engine killed by SIGTERM reported success")
		}
		if time.Since(start) > 10*time.Second {
			t.Fatalf("engine outlived the forwarded signal")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("passthrough did not return: the signal was not forwarded to the engine")
	}
}
