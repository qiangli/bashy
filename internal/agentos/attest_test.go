// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/yoke/pkg/craft"
	coreskills "github.com/qiangli/yoke/pkg/skills"
)

// readAttest reads the ledger back the way `bashy craft history` does — the
// existing read path, not a test-private parser — and returns one skill's
// observations in ledger order.
func readAttest(t *testing.T, store, name string) []craft.Observation {
	t.Helper()
	l, err := craft.ReadLedger(store)
	if err != nil {
		t.Fatal(err)
	}
	if l.Malformed != 0 {
		t.Fatalf("%d malformed receipts", l.Malformed)
	}
	var out []craft.Observation
	for _, o := range l.Observations {
		if o.Name == name {
			out = append(out, o)
		}
	}
	return out
}

func statusOf(t *testing.T, o craft.Observation) int {
	t.Helper()
	if o.Status == nil {
		t.Fatalf("receipt for %s carries no status: %+v", o.Name, o)
	}
	return *o.Status
}

// The Sprint 203 fixture, replayed with the store pointed at a private
// directory, attests every call it makes: success, a failed precondition, a
// failed postcondition, a guard-denied body, a plain failure and a yield —
// each one receipt, in call order, with the clause verdicts and the status
// that the transcript already pins. The yield is a handoff, not a completion.
func TestAttestFixtureLedger(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "test", "contracts", "agentic-boundary.bpp"))
	if err != nil {
		t.Fatal(err)
	}
	store := t.TempDir()
	runErr, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, string(src), map[string]string{"BASHY_AUDIT": "0", "BASHY_SKILLS_DIR": store})
	if runErr != nil {
		t.Fatalf("fixture exited non-zero: %v (stderr %q)", runErr, errOut.String())
	}
	if strings.Contains(errOut.String(), "bashy: attest:") {
		t.Fatalf("attest spoke: %q", errOut.String())
	}

	// summarize is called six times; every call is exactly one receipt.
	got := readAttest(t, store, "summarize")
	want := []struct {
		status int
		valid  bool
		passed []string
		failed []string
	}{
		{0, true, []string{`require:test -n "$1"`, `ensure:test "$1" != lie`}, nil},       // ok
		{3, false, nil, []string{`require:test -n "$1"`}},                                 // "": body never ran, ensure never judged
		{3, false, []string{`require:test -n "$1"`}, []string{`ensure:test "$1" != lie`}}, // lie
		{1, false, []string{`require:test -n "$1"`}, nil},                                 // write: guard denied inside the body
		{1, false, []string{`require:test -n "$1"`}, nil},                                 // fail
		{coreskills.AttestYield, false, []string{`require:test -n "$1"`}, nil},            // yield
	}
	if len(got) != len(want) {
		t.Fatalf("summarize receipts = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		o := got[i]
		if statusOf(t, o) != w.status || o.Valid != w.valid || !equalStrings(o.Passed, w.passed) || !equalStrings(o.Failed, w.failed) {
			t.Errorf("summarize call %d: status %d valid %v passed %q failed %q; want %d %v %q %q",
				i, *o.Status, o.Valid, o.Passed, o.Failed, w.status, w.valid, w.passed, w.failed)
		}
		if o.Identity != "fn:summarize" || o.ContextKey == "" || !strings.HasPrefix(o.StoreRevision, craft.RevVersion+":") {
			t.Errorf("summarize call %d: identity %q coordinate %q revision %q", i, o.Identity, o.ContextKey, o.StoreRevision)
		}
	}
	if !got[5].Yielded() || got[5].Valid || len(got[5].Failed) != 0 {
		t.Fatalf("the yield read back as a completion or a failure: %+v", got[5])
	}
	// The read side's own summary agrees: a yield is in Runs and in neither
	// Passed nor Failed.
	l, _ := craft.ReadLedger(store)
	if s := l.ForSkill("summarize"); s.Runs != 6 || s.Passed != 1 || s.Failed != 4 || s.Yielded != 1 {
		t.Fatalf("summary = %+v", s)
	}

	// A shell function and a typed function attest the same way; the
	// postcondition's STATUS/RESULT binding is visible in the check text.
	if got := readAttest(t, store, "produce"); len(got) != 2 || !got[0].Valid || got[1].Valid || statusOf(t, got[1]) != 3 {
		t.Fatalf("produce receipts = %+v", got)
	}
	if got := readAttest(t, store, "twice"); len(got) != 2 || !got[0].Valid || got[1].Valid ||
		!equalStrings(got[1].Failed, []string{`ensure:test "$RESULT" -gt 0`}) {
		t.Fatalf("twice receipts = %+v", got)
	}

	// The store revision moves as the ledger grows: the last receipt names a
	// larger store than the first, and each names the store BEFORE its own
	// append (the very first receipt sees an empty ledger set).
	first, last := l.Observations[0], l.Observations[len(l.Observations)-1]
	if first.StoreRevision == last.StoreRevision {
		t.Fatalf("store revision never moved: %q", first.StoreRevision)
	}
	if !strings.HasSuffix(first.StoreRevision, ".a0/0") {
		t.Fatalf("first receipt's revision should name an empty attest set: %q", first.StoreRevision)
	}
}

// An agentic{} function with no decorator of its own still completes into the
// ledger (advice puts the attest rung on it); a plain, undecorated,
// non-agentic function is an identity — no receipt, no ledger, no directory.
func TestAttestAgenticFunctionWithoutDecorators(t *testing.T) {
	store := t.TempDir()
	script := `agentic function ask() { return 6; }
agentic function finish() { echo "did $1"; }
function plain() { echo "plain $1"; }
agentic {
    ask;   echo "ask -> $?"
    finish a; echo "finish -> $?"
}
plain b
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"BASHY_SKILLS_DIR": store})
	if err != nil {
		t.Fatalf("script failed: %v (stderr %q)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "ask -> 6\ndid a\nfinish -> 0\nplain b") {
		t.Fatalf("stdout = %q", out.String())
	}
	if got := readAttest(t, store, "ask"); len(got) != 1 || !got[0].Yielded() || got[0].Valid {
		t.Fatalf("ask receipts = %+v", got)
	}
	if got := readAttest(t, store, "finish"); len(got) != 1 || !got[0].Valid || statusOf(t, got[0]) != 0 || len(got[0].Passed) != 0 {
		t.Fatalf("finish receipts = %+v", got)
	}
	if got := readAttest(t, store, "plain"); len(got) != 0 {
		t.Fatalf("an undecorated function attested: %+v", got)
	}
}

// One call, one receipt, whatever it is decorated with: the outermost native
// rung appends, the inner ones contribute. A decorated callee inside a
// decorated body is its own call and its own receipt. Under @retry the last
// attempt's verdict is the one recorded.
func TestAttestOneReceiptPerCall(t *testing.T) {
	store := t.TempDir()
	script := `@trace()
@guard(effects: "read")
@require('test -n "$1"')
@ensure('test "$STATUS" -eq 0')
function outer() { inner "$1"; }

@require('test -n "$1"')
function inner() { echo "inner $1"; }

n=0
@retry(n: 3, backoff: "0s")
@ensure('test "$STATUS" -eq 0')
function flaky() { n=$((n + 1)); test "$n" -ge 2; }

outer x
flaky; echo "flaky -> $? after $n"
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"BASHY_SKILLS_DIR": store})
	if err != nil {
		t.Fatalf("script failed: %v (stderr %q)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "flaky -> 0 after 2") {
		t.Fatalf("stdout = %q", out.String())
	}
	if got := readAttest(t, store, "outer"); len(got) != 1 || !got[0].Valid ||
		!equalStrings(got[0].Passed, []string{`require:test -n "$1"`, `ensure:test "$STATUS" -eq 0`}) {
		t.Fatalf("outer receipts = %+v", got)
	}
	if got := readAttest(t, store, "inner"); len(got) != 1 || !got[0].Valid || !equalStrings(got[0].Passed, []string{`require:test -n "$1"`}) {
		t.Fatalf("inner receipts = %+v", got)
	}
	// flaky: attempt 1 broke the postcondition (status 1, then 3), attempt 2
	// held. One receipt, the final verdict.
	if got := readAttest(t, store, "flaky"); len(got) != 1 || !got[0].Valid || statusOf(t, got[0]) != 0 ||
		!equalStrings(got[0].Passed, []string{`ensure:test "$STATUS" -eq 0`}) || len(got[0].Failed) != 0 {
		t.Fatalf("flaky receipts = %+v", got)
	}
}

// Bash OFF is an identity: a plain-Bash script never registers a decorator or
// consults advice, so a function completing under it leaves no ledger at all.
// BASHY_ATTEST=0 is the same silence under Bash++.
func TestAttestOffIsSilent(t *testing.T) {
	for _, tc := range []struct {
		name string
		lang syntax.LangVariant
		src  string
		env  map[string]string
	}{
		{"bash-off", syntax.LangBash, "function f() { echo \"f $1\"; }\nf a\n", nil},
		{"switched-off", syntax.LangBashPP, "@require('test -n \"$1\"')\nagentic function f() { echo \"f $1\"; }\nagentic { f a; }\n", map[string]string{"BASHY_ATTEST": "0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := t.TempDir()
			env := map[string]string{"BASHY_SKILLS_DIR": store}
			for k, v := range tc.env {
				env[k] = v
			}
			err, out, errOut := runDecorated(t, context.Background(), tc.lang, tc.src, env)
			if err != nil || !strings.Contains(out.String(), "f a") {
				t.Fatalf("script: %v stdout %q stderr %q", err, out.String(), errOut.String())
			}
			if _, statErr := os.Stat(craft.AttestDir(store)); !os.IsNotExist(statErr) {
				t.Fatalf("a ledger appeared: %v", statErr)
			}
		})
	}
}

// A store that cannot be written is spoken once, not per call, and never
// changes the call's own outcome.
func TestAttestUnwritableStoreSpeaksOnce(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	store := filepath.Join(t.TempDir(), "ro")
	if err := os.MkdirAll(store, 0o500); err != nil {
		t.Fatal(err)
	}
	script := "@require('true')\nfunction f() { echo \"f $1\"; }\nf a; f b\n"
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"BASHY_SKILLS_DIR": store})
	if err != nil || out.String() != "f a\nf b\n" {
		t.Fatalf("script: %v stdout %q", err, out.String())
	}
	if n := strings.Count(errOut.String(), "bashy: attest:"); n != 1 {
		t.Fatalf("spoken %d times: %q", n, errOut.String())
	}
}

// The advice seam puts the attest rung on an agentic registration and on
// nothing else; with attest off it adds nothing, and a broken rules file still
// refuses every call rather than attesting around the refusal.
func TestAttestAdviceRung(t *testing.T) {
	var warn bytes.Buffer
	cb := newAdviceCallback([]string{}, &warn, true)
	if specs := cb("f", "x.bpp", true); len(specs) != 1 || specs[0].ID != attestAdviceID || specs[0].Name != "attest" {
		t.Fatalf("agentic specs = %+v", specs)
	}
	if specs := cb("f", "x.bpp", false); len(specs) != 0 {
		t.Fatalf("non-agentic specs = %+v", specs)
	}
	if specs := newAdviceCallback([]string{}, &warn, false)("f", "x.bpp", true); len(specs) != 0 {
		t.Fatalf("attest off still advised: %+v", specs)
	}
	garbage := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(garbage, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if specs := newAdviceCallback([]string{"BASHY_ADVICE=" + garbage}, &warn, true)("f", "x.bpp", true); len(specs) != 1 || specs[0].ID != "invalid-policy" {
		t.Fatalf("broken rules with attest on = %+v", specs)
	}
	// The rung itself is a pass-through that refuses arguments.
	if err := attestDecorator(context.Background(), &nativeDecoratorCall{Next: func(context.Context) {}}, []interp.DecoratorArg{{Value: "x"}}); err == nil {
		t.Fatal("attest accepted an argument")
	}
}

// The sink resolves the store from the shell's env, off-switch first.
func TestAttestSinkResolution(t *testing.T) {
	if s := newAttestSink([]string{"BASHY_ATTEST=off", "BASHY_SKILLS_DIR=/x"}, nil); s != nil {
		t.Fatal("off switch ignored")
	}
	if s := newAttestSink([]string{"BASHY_HOME=/h"}, nil); s == nil || s.storeDir != filepath.Join("/h", "skills") {
		t.Fatalf("BASHY_HOME sink = %+v", s)
	}
	if s := newAttestSink([]string{"BASHY_HOME=/h", "BASHY_SKILLS_DIR=/s"}, nil); s == nil || s.storeDir != "/s" {
		t.Fatalf("BASHY_SKILLS_DIR sink = %+v", s)
	}
	var nilSink *attestSink
	fn := nilSink.attesting(func(context.Context, *nativeDecoratorCall, []interp.DecoratorArg) error { return nil })
	if err := fn(context.Background(), &nativeDecoratorCall{}, nil); err != nil {
		t.Fatal(err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
