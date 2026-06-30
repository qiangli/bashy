// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"io"
	"testing"
)

func TestRunCaptureMode(t *testing.T) {
	env, status := runCommand([]string{"sh", "-c", "printf out; printf err >&2; exit 3"}, true, nil, nil)
	if status != 3 || env.Exit != 3 {
		t.Errorf("exit = %d/%d, want 3", status, env.Exit)
	}
	if env.Stdout != "out" || env.Stderr != "err" {
		t.Errorf("captured stdout=%q stderr=%q, want out/err", env.Stdout, env.Stderr)
	}
	if env.Schema != runSchemaVersion {
		t.Errorf("schema = %q", env.Schema)
	}
}

func TestRunStreamMode(t *testing.T) {
	var out, errb bytes.Buffer
	env, status := runCommand([]string{"sh", "-c", "echo hi"}, false, &out, &errb)
	if status != 0 {
		t.Errorf("exit = %d, want 0", status)
	}
	if out.String() != "hi\n" {
		t.Errorf("live stdout = %q, want hi", out.String())
	}
	if env.Stdout != "" {
		t.Errorf("stream mode must not embed stdout, got %q", env.Stdout)
	}
	if env.DurationMs < 0 {
		t.Error("duration should be set")
	}
}

func TestRunSignaledIsNonLossy(t *testing.T) {
	env, status := runCommand([]string{"sh", "-c", "kill -KILL $$"}, true, nil, nil)
	if !env.Signaled {
		t.Error("a SIGKILL'd process should report signaled=true")
	}
	if status < 128 {
		t.Errorf("signaled status = %d, want >=128 (128+sig)", status)
	}
}

func TestRunMissingCommand(t *testing.T) {
	_, status := runCommand([]string{"definitely-not-a-real-cmd-xyz"}, false, io.Discard, io.Discard)
	if status != 127 {
		t.Errorf("missing command exit = %d, want 127", status)
	}
}
