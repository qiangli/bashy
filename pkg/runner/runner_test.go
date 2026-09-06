package runner

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
	if result.SchemaVersion != SchemaVersion || result.Outcome != OutcomeOK || result.Signaled || result.Signal != "" || result.DurationMs < 0 {
		t.Fatalf("envelope = %#v", result)
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
	if result.ExitCode != 17 || result.Outcome != OutcomeExit {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunDistinguishesSignalFromSameNumericExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signals are a Unix process facility")
	}
	signaled := Run(context.Background(), Request{Script: "/bin/sh -c 'kill -TERM $$'"})
	if signaled.ExitCode != 143 || !signaled.Signaled || signaled.Signal != "SIGTERM" || signaled.Outcome != OutcomeSignal {
		t.Fatalf("signaled result = %#v", signaled)
	}
	exited := Run(context.Background(), Request{Script: "/bin/sh -c 'exit 143'"})
	if exited.ExitCode != 143 || exited.Signaled || exited.Signal != "" || exited.Outcome != OutcomeExit {
		t.Fatalf("explicit-exit result = %#v", exited)
	}
}

func TestPreflightUsesSameRequestWithoutEffects(t *testing.T) {
	dir := t.TempDir()
	result := Preflight(context.Background(), Request{
		Script: "mkdir created; printf '%s' \"$HARNESS_VALUE\"; printf warning >&2",
		Dir:    dir,
		Env:    []string{"PATH=/bin:/usr/bin", "HARNESS_VALUE=request-env"},
	})
	if result.ExitCode != 0 || result.Outcome != OutcomeOK {
		t.Fatalf("result = %#v", result)
	}
	if result.Stdout != "request-env" || !strings.Contains(result.Stderr, "mkdir created") {
		t.Fatalf("preflight streams = stdout %q, stderr %q", result.Stdout, result.Stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "created")); !os.IsNotExist(err) {
		t.Fatalf("preflight created directory: %v", err)
	}
}

func TestRunDoesNotConsultCLIDryRunFlag(t *testing.T) {
	option := flag.Lookup("dryrun")
	if option == nil {
		t.Fatal("AgentOS dry-run option is not registered")
	}
	old := option.Value.String()
	if err := flag.Set("dryrun", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = flag.Set("dryrun", old) })

	dir := t.TempDir()
	result := Run(context.Background(), Request{Script: "mkdir created", Dir: dir})
	if result.Outcome != OutcomeOK {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "created")); err != nil {
		t.Fatalf("Run was affected by CLI --dry-run state: %v", err)
	}
}

func TestRunClassifiesCancelledContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses the portable Unix sleep command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	result := Run(ctx, Request{Script: "sleep 5", Env: []string{"PATH=/bin:/usr/bin"}})
	if result.Outcome != OutcomeTimeout {
		t.Fatalf("result = %#v", result)
	}
}
