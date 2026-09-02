package runner

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRunExecutesBashyInProcess(t *testing.T) {
	dir := t.TempDir()
	result := Run(context.Background(), Request{
		Script: "printf '%s' \"$PWD\"; printf problem >&2",
		Dir:    dir,
		Env:    []string{"PATH=/bin:/usr/bin"},
	})
	if result.ExitCode != 0 || result.Stdout != filepath.Clean(dir) || result.Stderr != "problem" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunBoundsOutput(t *testing.T) {
	result := Run(context.Background(), Request{Script: "printf 123456", MaxOutputChars: 4})
	if result.Stdout != "1234" || !result.StdoutTruncated {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunReportsExitStatus(t *testing.T) {
	result := Run(context.Background(), Request{Script: "exit 17"})
	if result.ExitCode != 17 {
		t.Fatalf("exit code = %d, stderr = %q", result.ExitCode, result.Stderr)
	}
}
