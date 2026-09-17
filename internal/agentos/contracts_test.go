// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// TestContractsAgenticBoundaryFixture runs the Sprint 203 fixture in-process,
// wired exactly as the agentic shell is, and pins its transcript — the same
// file test/contracts/run.sh checks on the built and installed binary.
func TestContractsAgenticBoundaryFixture(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "contracts")
	src, err := os.ReadFile(filepath.Join(dir, "agentic-boundary.bpp"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(dir, "agentic-boundary.expected"))
	if err != nil {
		t.Fatal(err)
	}
	// One buffer: the fixture's stdout and stderr interleave in the transcript
	// (the failure line names the clause right where the call happened), and
	// what follows a per-call message is the caller's own echo of $?.
	runErr, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, string(src), map[string]string{"BASHY_AUDIT": "0"})
	if runErr != nil {
		t.Fatalf("fixture exited non-zero: %v (stderr %q)", runErr, errOut.String())
	}
	// Lines pinned in the transcript, in order; stderr lines are checked to be
	// exactly the contract messages (the fixture prints nothing else there).
	var wantOut, wantErr []string
	for _, ln := range strings.Split(strings.TrimSpace(string(want)), "\n") {
		if strings.Contains(ln, " failed: ") {
			wantErr = append(wantErr, ln)
		} else {
			wantOut = append(wantOut, ln)
		}
	}
	if got := strings.TrimSpace(out.String()); got != strings.Join(wantOut, "\n") {
		t.Errorf("stdout:\n%s\nwant:\n%s", got, strings.Join(wantOut, "\n"))
	}
	if got := strings.TrimSpace(errOut.String()); got != strings.Join(wantErr, "\n") {
		t.Errorf("stderr:\n%s\nwant:\n%s", got, strings.Join(wantErr, "\n"))
	}
}

func TestContractRequireSkipsBodyExit3(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	script := `@require('test -n "$1"')
function f() { touch "$2"; }
f "" "$MARKER"
`
	err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"MARKER": marker})
	if status, ok := exitStatusOf(err); !ok || status != 3 {
		t.Fatalf("want exit 3, got %v (stderr %q)", err, errOut.String())
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatal("body ran despite a failed precondition")
	}
	if got := errOut.String(); got != "f: precondition failed: test -n \"$1\"\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestContractEnsureJudgesCompletedBodyOnly(t *testing.T) {
	// A yield (6) and a plain failure (1) keep their own status; the
	// postcondition never runs on them. A clean body that breaks the
	// postcondition exits 3.
	for _, tc := range []struct {
		name   string
		body   string
		status int
		msg    bool
	}{
		{"yield propagates untouched", "return 6", 6, false},
		{"failure keeps its status", "return 1", 1, false},
		{"clean body, broken postcondition", "return 0", 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := "@ensure('false')\nfunction f() { " + tc.body + "; }\nf\n"
			err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
			if status, ok := exitStatusOf(err); !ok || status != tc.status {
				t.Fatalf("want exit %d, got %v", tc.status, err)
			}
			if got := strings.Contains(errOut.String(), "postcondition failed: false"); got != tc.msg {
				t.Errorf("postcondition message present=%v, want %v (stderr %q)", got, tc.msg, errOut.String())
			}
		})
	}
}

func TestContractEnsureSeesStatusAndResult(t *testing.T) {
	script := `@ensure('test "$STATUS" = 0 && test "$RESULT" = 6')
func triple(n int) int { return n * 3 }
x := triple(2)
echo "x=$x"
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if err != nil {
		t.Fatalf("want pass, got %v (stderr %q)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "x=6") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestContractDecoratorArguments(t *testing.T) {
	for _, tc := range []struct{ name, script, want string }{
		{"require needs a check", "@require()\nfunction f() { :; }\nf\n", "require takes at least one check"},
		{"ensure refuses named args", "@ensure(check: 'true')\nfunction f() { :; }\nf\n", "positional checks only"},
		{"empty check", "@require('  ')\nfunction f() { :; }\nf\n", "empty check"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, tc.script, nil)
			if err == nil {
				t.Fatal("want a decorator diagnostic")
			}
			if !strings.Contains(errOut.String(), tc.want) {
				t.Errorf("stderr %q does not mention %q", errOut.String(), tc.want)
			}
		})
	}
}

func TestContractDecoratorsAreSourceOnly(t *testing.T) {
	c := &nativeDecoratorCall{Name: "f", Advised: "rule-1"}
	for name, fn := range contractDecorators(io.Discard) {
		err := fn(context.Background(), c, []interp.DecoratorArg{{Value: "true"}})
		if err == nil || !strings.Contains(err.Error(), "source-only") {
			t.Errorf("%s advised: err = %v, want source-only refusal", name, err)
		}
	}
}
