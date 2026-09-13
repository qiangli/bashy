package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shipped `bashy dag` is the pkg/dag root WITH the capacity subcommand
// mounted. Sprint 163 found it exiting 1 with zero bytes on both streams for
// every target: cobra rejected the target as an unknown subcommand and
// SilenceErrors + a bare exit-code mapping hid the message. These tests drive
// the exact dispatch shape bashy ships.

func writeTestDAG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	md := "## Tasks\n\n### hello\nSay hi.\n\n```bash\necho hi-from-dispatch\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "DAG.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "DAG.md")
}

func TestDagDispatchRunsATargetWithCapacityMounted(t *testing.T) {
	path := writeTestDAG(t)
	var out, errOut bytes.Buffer
	code := runDagDispatch([]string{"--file", path, "hello"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("dag hello exit = %d\nstdout=%s\nstderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "hi-from-dispatch") {
		t.Fatalf("target body did not run\nstdout=%s\nstderr=%s", out.String(), errOut.String())
	}
}

func TestDagDispatchDryRunWithCapacityMounted(t *testing.T) {
	path := writeTestDAG(t)
	var out, errOut bytes.Buffer
	if code := runDagDispatch([]string{"--file", path, "-n", "hello"}, &out, &errOut); code != 0 {
		t.Fatalf("dag -n hello exit = %d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("plan did not name the target:\n%s", out.String())
	}
}

// A non-zero exit with an empty stderr is the absence-of-evidence failure
// mode; an error cobra swallows before RunE must still be reported.
func TestDagDispatchNeverExitsSilently(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runDagDispatch([]string{"--no-such-flag"}, &out, &errOut)
	if code == 0 {
		t.Fatal("an unknown flag must fail")
	}
	if !strings.Contains(errOut.String(), "no-such-flag") {
		t.Fatalf("the swallowed cobra error was not reported: stderr=%q", errOut.String())
	}
}

// dag's own errors are already emitted as envelopes by pkg/dag; they must not
// be printed a second time by the dispatcher.
func TestDagDispatchDoesNotDoubleReportDagErrors(t *testing.T) {
	path := writeTestDAG(t)
	var out, errOut bytes.Buffer
	code := runDagDispatch([]string{"--file", path, "nosuch"}, &out, &errOut)
	if code == 0 {
		t.Fatal("unknown target must fail")
	}
	if n := strings.Count(errOut.String(), "nosuch"); n != 1 {
		t.Fatalf("unknown target reported %d times, want exactly once:\n%s", n, errOut.String())
	}
	if strings.Contains(errOut.String(), "bashy dag:") {
		t.Fatalf("a *dag.Error must not also be printed by the dispatcher:\n%s", errOut.String())
	}
}
