// Copyright (c) 2026, the bashy authors.
// See LICENSE for licensing information.

// Sprint 221, Story #580 (Story-ID c19cb824e6bd), B27b integration tests:
// defineSessionHandler wiring HandlerContext.DescribeValue (../sh, cad5a8e3)
// into `bashy define`. Fixture provenance is pinned in
// ../sh/plan-b27-timed-introspection.md (PowerShell Get-Member, MIT;
// this repo's own decorator/adapter ordering tests, BSD-3-Clause).
package agentos

import (
	"context"
	"io"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// defineSessionScript declares one of each shape DescribeValue answers for,
// exactly like ../sh's own describeSessionSrc fixture.
const defineSessionScript = `type Point struct { X int; Y int; secret string; Tags []string }
n := 42
s := "hello"
p := Point{X: 1, Y: 2}
ptr := &p
var nilptr *Point
xs := []int{1, 2, 3}
m := make(map[string]int)
m["a"] = 1
ch := make(chan int, 1)
var e error
classic=plain
`

// runDefineSession runs defineSessionScript followed by one `command bashy
// define <tail>` through a runner wired with ONLY defineSessionHandler ahead
// of a spy stand-in for "next". This exercises the exact production rung
// (agentos.go's wireExec installs the very same defineSessionHandler())
// without depending on a real `bashy` binary being on PATH — the spy IS the
// fallback, so a miss is provable by inspecting what reached it rather than
// by an actual process exec succeeding or failing.
func runDefineSession(t *testing.T, tail string) (stdout string, nextCalled bool, nextArgs []string) {
	t.Helper()
	var out strings.Builder
	script := defineSessionScript + "command bashy define " + tail + "\n"
	prog, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(script), "define_session_test.sh")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	spy := func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(_ context.Context, args []string) error {
			nextCalled = true
			nextArgs = args
			return nil
		}
	}
	runner, err := interp.New(
		interp.Lang(syntax.LangBashPP),
		interp.Env(nil),
		interp.StdIO(nil, &out, io.Discard),
		interp.ExecHandlers(defineSessionHandler(), spy),
	)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if err := runner.Run(context.Background(), prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String(), nextCalled, nextArgs
}

func TestDefineSessionScalar(t *testing.T) {
	out, called, _ := runDefineSession(t, "n")
	if called {
		t.Fatalf("next was called; want the session rung to answer directly")
	}
	want := "n  (session scalar)\n  type: int\n  value: 42\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestDefineSessionClassicVariable(t *testing.T) {
	// A classic shell variable answers through the same call — no Bash#
	// typed cell involved — so `define` needs one seam, not two.
	out, called, _ := runDefineSession(t, "classic")
	if called {
		t.Fatal("next was called; want the session rung to answer directly")
	}
	want := "classic  (session scalar)\n  type: string\n  value: plain\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestDefineSessionRecordFieldsAndRedaction(t *testing.T) {
	out, called, _ := runDefineSession(t, "p")
	if called {
		t.Fatal("next was called; want the session rung to answer directly")
	}
	for _, want := range []string{
		"p  (session record)\n",
		"type: Point\n",
		"X  int = 1\n",
		"Y  int = 2\n",
		// private field: name and type only, value withheld.
		"secret  string  ‹redacted›\n",
		// composite field: shape via its type alone, no value line.
		"Tags  []string\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output = %q, missing %q", out, want)
		}
	}
	if strings.Contains(out, "hello world") { // sanity: never invents a value
		t.Fatalf("output = %q, must not fabricate a secret value", out)
	}
}

func TestDefineSessionList(t *testing.T) {
	out, called, _ := runDefineSession(t, "xs")
	if called {
		t.Fatal("next was called; want the session rung to answer directly")
	}
	want := "xs  (session list)\n  type: []int\n  len:  3\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestDefineSessionMap(t *testing.T) {
	out, called, _ := runDefineSession(t, "m")
	if called {
		t.Fatal("next was called; want the session rung to answer directly")
	}
	want := "m  (session map)\n  type: map[string]int\n  len:  1\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestDefineSessionHandle(t *testing.T) {
	// A channel is opaque: identity is the answer, the value is withheld.
	out, called, _ := runDefineSession(t, "ch")
	if called {
		t.Fatal("next was called; want the session rung to answer directly")
	}
	if !strings.HasPrefix(out, "ch  (session handle)\n  type: chan") {
		t.Fatalf("output = %q", out)
	}
	if !strings.Contains(out, "‹redacted› (opaque handle)") {
		t.Fatalf("output = %q, want the value withheld", out)
	}

	// A non-nil pointer is a handle too: withheld the same way.
	out, called, _ = runDefineSession(t, "ptr")
	if called {
		t.Fatal("next was called; want the session rung to answer directly")
	}
	want := "ptr  (session handle)\n  type: *Point\n  value: ‹redacted› (opaque handle)\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestDefineSessionNil(t *testing.T) {
	// A nil pointer: nil said explicitly, never confused with redaction.
	out, called, _ := runDefineSession(t, "nilptr")
	if called {
		t.Fatal("next was called; want the session rung to answer directly")
	}
	want := "nilptr  (session handle)\n  type: *Point\n  value: nil\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}

	// A nil interface value.
	out, called, _ = runDefineSession(t, "e")
	if called {
		t.Fatal("next was called; want the session rung to answer directly")
	}
	if !strings.Contains(out, "e  (session nil)\n") || !strings.Contains(out, "value: nil\n") {
		t.Fatalf("output = %q", out)
	}
}

// TestDefineSessionIsNotAnEvaluator proves the boundary: anything shaped
// like a selector, an index, an expression, or a flag falls straight
// through to "next" unchanged — never evaluated, never partially answered.
func TestDefineSessionIsNotAnEvaluator(t *testing.T) {
	cases := []string{"'p.X'", "'xs[0]'", "'m[a]'", "--json n", "--list-kinds", "n --json"}
	for _, tail := range cases {
		out, called, args := runDefineSession(t, tail)
		if out != "" {
			t.Errorf("tail %q: output = %q, want none (must fall through)", tail, out)
		}
		if !called {
			t.Errorf("tail %q: next was NOT called; want the ordinary fallback", tail)
		}
		if len(args) == 0 || args[0] != "bashy" || args[1] != "define" {
			t.Errorf("tail %q: next args = %v, want the untouched define invocation", tail, args)
		}
	}
}

// TestDefineSessionFallback proves the "before ordinary lexicon fallback"
// half of the contract: a name that is NOT live in this session (unset, or
// only the projected registries would know it) falls straight through to
// "next" — the exact same fork-based front door `bashy define` has always
// used — rather than being answered (or refused) by this rung.
func TestDefineSessionFallback(t *testing.T) {
	out, called, args := runDefineSession(t, "nosuchvar")
	if out != "" {
		t.Fatalf("output = %q, want none — an unset name is not this rung's to answer", out)
	}
	if !called {
		t.Fatal("next was NOT called; want the ordinary lexicon fallback")
	}
	want := []string{"bashy", "define", "nosuchvar"}
	if len(args) != len(want) {
		t.Fatalf("next args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("next args = %v, want %v", args, want)
		}
	}
}

// TestDefineSessionCandidateNonSelfInvocation proves this rung only ever
// touches a self re-exec of `define`: an ordinary external command — even
// one that happens to be named "define" or take a look-alike argument list
// — is left completely alone.
func TestDefineSessionCandidateNonSelfInvocation(t *testing.T) {
	cases := [][]string{
		{"git", "define", "n"},           // not bashy
		{"bashy", "commands", "n"},       // not define
		{"bashy", "define"},              // no term (arity handled by cobra)
		{"bashy", "define", "n", "--json"}, // extra operand
	}
	for _, args := range cases {
		if _, ok := defineSessionCandidate(args); ok {
			t.Errorf("defineSessionCandidate(%v) matched; want false", args)
		}
	}
	if name, ok := defineSessionCandidate([]string{"bashy.real", "define", "n"}); !ok || name != "n" {
		t.Errorf("defineSessionCandidate for the native-launcher pair = (%q, %v), want (\"n\", true)", name, ok)
	}
	if name, ok := defineSessionCandidate([]string{"/usr/local/bin/bashy", "define", "n"}); !ok || name != "n" {
		t.Errorf("defineSessionCandidate for an absolute self path = (%q, %v), want (\"n\", true)", name, ok)
	}
}

// TestDefineSessionDirectFrontDoorUnaffected pins that a direct front-door
// `bashy define` — Dispatch's own case in agentos.go, reached with no
// running Bash# session at all — never goes anywhere near this rung: it
// builds the ordinary lexicon.NewDefineCmd() exactly as before.
func TestDefineSessionDirectFrontDoorUnaffected(t *testing.T) {
	cmd := newDefineCmd()
	if len(cmd.Commands()) != 0 {
		t.Fatalf("bashy define grew a subcommand: %v", cmd.Commands())
	}
	if cmd.Use != "define <term>" {
		t.Fatalf("Use = %q, want the unmodified lexicon.NewDefineCmd() front door", cmd.Use)
	}
}
