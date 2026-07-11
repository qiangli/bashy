// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"slices"
	"testing"
)

func TestRunCaptureMode(t *testing.T) {
	env, status := runCommand(testHelperCommand("capture"), true, false, nil, nil)
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
	env, status := runCommand(testHelperCommand("stream"), false, false, &out, &errb)
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
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX signals on Windows; signaled-status encoding is unix-only")
	}
	env, status := runCommand([]string{"sh", "-c", "kill -KILL $$"}, true, false, nil, nil)
	if !env.Signaled {
		t.Error("a SIGKILL'd process should report signaled=true")
	}
	if status < 128 {
		t.Errorf("signaled status = %d, want >=128 (128+sig)", status)
	}
}

func TestRunMissingCommand(t *testing.T) {
	_, status := runCommand([]string{"definitely-not-a-real-cmd-xyz"}, false, false, io.Discard, io.Discard)
	if status != 127 {
		t.Errorf("missing command exit = %d, want 127", status)
	}
}

func TestRunMarksChildAgentic(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "0")
	env, status := runCommand(testHelperCommand("env"), true, false, nil, nil)
	if status != 0 {
		t.Fatalf("exit = %d, want 0", status)
	}
	if env.Stdout != "1" {
		t.Fatalf("BASHY_AGENTIC in child = %q, want 1", env.Stdout)
	}
}

func TestRunCommandEnvSetsAgentic(t *testing.T) {
	t.Parallel()

	got := runCommandEnv([]string{"PATH=/bin", "BASHY_AGENTIC=0", "BASHY_AGENTIC=old"})
	want := []string{"PATH=/bin", "BASHY_AGENTIC=1", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}
	if len(got) != len(want) {
		t.Fatalf("wrong env length: want %d, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("env[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestRunCommandEnvPreservesExplicitGitPromptEnv(t *testing.T) {
	t.Parallel()

	got := runCommandEnv([]string{"GIT_TERMINAL_PROMPT=1", "GCM_INTERACTIVE=always"})
	want := []string{"GIT_TERMINAL_PROMPT=1", "GCM_INTERACTIVE=always", "BASHY_AGENTIC=1"}
	if len(got) != len(want) {
		t.Fatalf("wrong env length: want %d, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("env[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestRunCheckBlocksBadScript(t *testing.T) {
	dir := t.TempDir()
	script := dir + "/bad.sh"
	if err := os.WriteFile(script, []byte("definitely-not-a-real-cmd-xyz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env, status := runCommand([]string{script}, true, true, nil, nil)
	if status != 1 || env.Exit != 1 {
		t.Fatalf("status=%d exit=%d, want 1", status, env.Exit)
	}
	if env.Check == nil || env.Check.Summary.Errors == 0 {
		t.Fatalf("missing check errors in envelope: %#v", env.Check)
	}
}

func TestRunCommandHelper(t *testing.T) {
	i := slices.Index(os.Args, "--")
	if i < 0 || i+1 >= len(os.Args) {
		return
	}
	switch os.Args[i+1] {
	case "capture":
		os.Stdout.WriteString("out")
		os.Stderr.WriteString("err")
		os.Exit(3)
	case "stream":
		os.Stdout.WriteString("hi\n")
		os.Exit(0)
	case "env":
		os.Stdout.WriteString(os.Getenv("BASHY_AGENTIC"))
		os.Exit(0)
	}
	os.Exit(2)
}

func testHelperCommand(name string) []string {
	return []string{os.Args[0], "-test.run=^TestRunCommandHelper$", "--", name}
}
