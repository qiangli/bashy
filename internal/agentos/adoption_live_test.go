// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/coreutils/pkg/nudge"
	"github.com/qiangli/yoke/pkg/execlog"
)

func TestAdoptionUsesFromArgv(t *testing.T) {
	cases := []struct {
		argv []string
		want []string
	}{
		{[]string{"bashy", "lens", "-t", "md", "a.pdf"}, []string{"adopt:lens"}},
		{[]string{"lens", "-f", "json", "-t", "yaml"}, []string{"adopt:lens"}},
		{[]string{"/usr/local/bin/bashy", "adapt", "--json", "--", "git", "status"}, []string{"adopt:adapt", "adopt:option=--json"}},
		{[]string{"ls", "--fields=name,size", "--dry-run"}, []string{"adopt:option=--fields", "adopt:option=--dry-run"}},
		{[]string{"ls", "-l"}, nil},
		{[]string{"grep", "--jsonx", "lens"}, nil},     // near-miss flag, lens as an operand
		{[]string{"bashy", "-c", "lens -t md x"}, nil}, // bashy on itself, not a verb
	}
	for _, tc := range cases {
		if got := adoptionUses(tc.argv); !slices.Equal(got, tc.want) {
			t.Errorf("adoptionUses(%q) = %q, want %q", tc.argv, got, tc.want)
		}
	}
}

// The forms come from the real rule texts, so a reworded hint that loses its
// executable shape fails here rather than silently never counting as followed.
func TestUsesSuggestionAgainstRealHints(t *testing.T) {
	cdHint := nudge.Suggest([]string{"cd", "/tmp"}, true)
	grepHint := nudge.Suggest([]string{"grep", "-r", "x", "."}, false)
	zipHint := SuggestDeclared([]string{"unzip", "-l", "a.zip"}, false)
	adaptHint := SuggestDeclared([]string{"git", "--json"}, false)
	for _, h := range []string{cdHint, grepHint, zipHint, adaptHint} {
		if h == "" {
			t.Fatal("a rule this test relies on no longer fires")
		}
	}
	cases := []struct {
		tool, hint string
		argv       []string
		want       bool
	}{
		{"cd", cdHint, []string{"awd", "/tmp", "--", "ls"}, true},
		{"cd", cdHint, []string{"cd", "/tmp"}, false},
		{"grep", grepHint, []string{"grep", "-r", "--agentic", "x", "."}, true},
		{"grep", grepHint, []string{"bashy", "ast", "refs", "Foo"}, true},
		{"grep", grepHint, []string{"grep", "-r", "x", "."}, false},
		{"grep", grepHint, []string{"ast", "map"}, true},
		{"grep", grepHint, []string{"ast", "symbols"}, false},         // not what the hint names
		{"grep", grepHint, []string{"find", ".", "--agentic"}, false}, // flag-only form binds to grep
		{"unzip", zipHint, []string{"bashy", "lens", "-t", "listing", "a.zip"}, true},
		{"unzip", zipHint, []string{"unzip", "-l", "a.zip"}, false},
		{"git", adaptHint, []string{"bashy", "adapt", "--json", "--", "git", "status"}, true},
		{"git", adaptHint, []string{"bashy", "adapt", "--explain", "--", "git", "status"}, false},
	}
	for _, tc := range cases {
		got := usesSuggestion(tc.argv, suggestionForms(tc.tool, tc.hint))
		if got != tc.want {
			t.Errorf("%s hint, next %q: followed=%v, want %v (forms %+v)", tc.tool, tc.argv, got, tc.want, suggestionForms(tc.tool, tc.hint))
		}
	}
}

// call is one agent tool call: one bashy process with its own tally, nudger
// and recorder, sharing the episode's pending file with the other calls.
type call struct {
	live *adoptionLive
	nd   *nudger
	h    interp.ExecHandlerFunc
}

func newCall(t *testing.T, rec *recorder, agent, pending string, pid int) *call {
	t.Helper()
	live := newAdoptionLive(agent, pending)
	live.pid, live.ppid = pid, 1
	r := *rec
	r.adopt = live
	nd := &nudger{agent: true, mem: newTestMemory(t), w: &bytes.Buffer{}, live: live}
	inner := func(context.Context, []string) error { return nil }
	return &call{live: live, nd: nd, h: execHistHandler(&r)(inner)}
}

// run is what the shell does for one simple command: audit (tally, then the
// hinter), then the exec chain, where the recorder writes the record.
func (c *call) run(argv ...string) {
	ev := interp.AuditEvent{Args: argv}
	c.live.observe(ev.Args, false)
	c.nd.onAudit(ev)
	_ = c.h(context.Background(), argv)
}

func (c *call) builtin(argv ...string) {
	ev := interp.AuditEvent{Args: argv, IsBuiltin: true}
	c.live.observe(ev.Args, true)
	c.nd.onAudit(ev)
}

func readEffects(t *testing.T, logDir string) [][]string {
	t.Helper()
	recs, _, err := execlog.Read(logDir, execlog.Query{})
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, r := range recs {
		out = append(out, r.Effects)
	}
	return out
}

func adoptAtoms(effects []string) []string {
	var out []string
	for _, e := range effects {
		if strings.HasPrefix(e, adoptPrefix) {
			out = append(out, e)
		}
	}
	return out
}

func TestHintFollowedOnlyByTheSameToolsNextCall(t *testing.T) {
	rec, logDir, _ := testRecorder(t)
	pending := filepath.Join(t.TempDir(), "hint-pending", "ep.json")

	// Call 1 shows the grep hint; a later command in the SAME call that uses
	// the suggestion cannot have seen it and must not count.
	c1 := newCall(t, rec, "claude", pending, 101)
	c1.run("grep", "-r", "x", ".")
	c1.run("grep", "-r", "--agentic", "x", ".")
	c1.run("grep", "-r", "y", ".") // budget: one hint per tool per session

	// Call 2 (next call, same agent tool) follows it.
	c2 := newCall(t, rec, "claude", pending, 102)
	c2.run("ls")
	c2.run("grep", "-r", "--agentic", "x", ".")
	c2.run("grep", "-r", "--agentic", "z", ".") // credited once

	// Call 3 is not the next call: the hint was consumed by call 2.
	c3 := newCall(t, rec, "claude", pending, 103)
	c3.run("grep", "-r", "--agentic", "x", ".")

	got := readEffects(t, logDir)
	want := [][]string{
		{"adopt:agent=claude", "adopt:hint-shown=grep"},
		nil,
		nil,
		nil,
		{"adopt:agent=claude", "adopt:hint-followed=grep"},
		nil,
		nil,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if a := adoptAtoms(got[i]); !slices.Equal(a, want[i]) {
			t.Errorf("record %d adopt atoms = %q, want %q", i, a, want[i])
		}
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Errorf("the pending hint must be consumed by the next call, stat err=%v", err)
	}
}

func TestHintNotFollowedAcrossToolsOrByUnrelatedNextCall(t *testing.T) {
	rec, logDir, _ := testRecorder(t)
	pending := filepath.Join(t.TempDir(), "ep.json")

	newCall(t, rec, "claude", pending, 201).run("grep", "-r", "x", ".")
	// A different agent tool's call never follows claude's hint.
	newCall(t, rec, "codex", pending, 202).run("grep", "-r", "--agentic", "x", ".")

	newCall(t, rec, "claude", pending, 203).run("grep", "-r", "x", ".")
	// The next call ignores the suggestion (and shows nothing new); a later
	// one cannot recover it.
	newCall(t, rec, "claude", pending, 204).run("ls")
	newCall(t, rec, "claude", pending, 205).run("grep", "-r", "--agentic", "x", ".")

	for i, eff := range readEffects(t, logDir) {
		for _, a := range eff {
			if strings.HasPrefix(a, adoptPrefix+"hint-followed") {
				t.Errorf("record %d credited a follow it must not: %q", i, eff)
			}
		}
	}
}

// A nested bashy inside the call that showed the hint is part of that call.
func TestHintNotFollowedByNestedChildOfShowingCall(t *testing.T) {
	rec, logDir, _ := testRecorder(t)
	pending := filepath.Join(t.TempDir(), "ep.json")
	newCall(t, rec, "claude", pending, 301).run("grep", "-r", "x", ".")
	child := newCall(t, rec, "claude", pending, 302)
	child.live.ppid = 301
	child.run("grep", "-r", "--agentic", "x", ".")
	for _, eff := range readEffects(t, logDir) {
		if slices.Contains(eff, adoptPrefix+"hint-followed=grep") {
			t.Fatalf("a child of the showing call was credited: %q", eff)
		}
	}
}

// Builtins have no record of their own: the cd hint and its awd follow land on
// the next dispatched command's record.
func TestBuiltinHintCarriedToNextRecord(t *testing.T) {
	rec, logDir, _ := testRecorder(t)
	pending := filepath.Join(t.TempDir(), "ep.json")
	c1 := newCall(t, rec, "opencode", pending, 401)
	c1.builtin("cd", "/tmp")
	c1.run("ls")
	c2 := newCall(t, rec, "opencode", pending, 402)
	c2.builtin("awd", "/tmp", "--", "ls")
	c2.run("ls")
	got := readEffects(t, logDir)
	if len(got) != 2 {
		t.Fatalf("want 2 records, got %q", got)
	}
	if a := adoptAtoms(got[0]); !slices.Equal(a, []string{"adopt:agent=opencode", "adopt:hint-shown=cd"}) {
		t.Errorf("first record = %q", a)
	}
	if a := adoptAtoms(got[1]); !slices.Equal(a, []string{"adopt:agent=opencode", "adopt:hint-followed=cd"}) {
		t.Errorf("second record = %q", a)
	}
}

// Without a tally (recording off for hints) argv-derived uses still land, and
// the atlas's own effect slice is never written through.
func TestRecorderUsesWithoutTallyKeepAtlasEffects(t *testing.T) {
	rec, logDir, _ := testRecorder(t)
	before := slices.Clone(effectsFor("ls"))
	inner := func(context.Context, []string) error { return nil }
	_ = execHistHandler(rec)(inner)(context.Background(), []string{"ls", "--json"})
	_ = execHistHandler(rec)(inner)(context.Background(), []string{"ls"})
	got := readEffects(t, logDir)
	if !slices.Contains(got[0], "adopt:option=--json") {
		t.Errorf("uses not recorded: %q", got[0])
	}
	if a := adoptAtoms(got[1]); a != nil {
		t.Errorf("plain ls must carry no adopt atoms, got %q", a)
	}
	if !slices.Equal(effectsFor("ls"), before) {
		t.Errorf("atlas effects mutated: %q -> %q", before, effectsFor("ls"))
	}
}

// Through the real wireExec chain: BASHY_HINTS=off shows no hint, records no
// hint atom and leaves nothing pending; uses are still recorded. (The hint line
// itself goes to the process stderr; nudge_test covers its emission.)
func TestAdoptionThroughWireExecHonorsHintsOff(t *testing.T) {
	for _, hints := range []string{"on", "off"} {
		t.Run(hints, func(t *testing.T) {
			dir := t.TempDir()
			logDir := filepath.Join(dir, "exec")
			t.Setenv("BASHY_AGENTIC", "1")
			t.Setenv("BASHY_HINTS", hints)
			t.Setenv("BASHY_EXECHIST", logDir)
			t.Setenv("BASHY_HINTS_STATE", filepath.Join(dir, "inbox-hints.json"))
			pending := adoptionPendingPath("wire-ep")
			live := newAdoptionLive("claude", pending)
			orig := processAdoption
			processAdoption = func() *adoptionLive { return live }
			t.Cleanup(func() { processAdoption = orig })
			if !execHistEnabled() {
				t.Skip("execution record not enabled under BASHY_AGENTIC=1 on this host")
			}

			tree := filepath.Join(dir, "tree")
			if err := os.MkdirAll(tree, 0o755); err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			r, err := interp.New(interp.Env(nil))
			if err != nil {
				t.Fatal(err)
			}
			for _, opt := range wireExec(nil, false, os.Environ(), nil, &out, &errOut, false) {
				if err := opt(r); err != nil {
					t.Fatal(err)
				}
			}
			script := "grep -r zzz_no_match " + tree + "; grep -r --json zzz_no_match " + tree + "; true\n"
			prog, perr := syntax.NewParser().Parse(strings.NewReader(script), "")
			if perr != nil {
				t.Fatal(perr)
			}
			_ = r.Run(context.Background(), prog)

			got := readEffects(t, logDir)
			var all []string
			for _, e := range got {
				all = append(all, adoptAtoms(e)...)
			}
			shown := slices.Contains(all, "adopt:hint-shown=grep")
			_, statErr := os.Stat(pending)
			if hints == "on" {
				if !shown || statErr != nil {
					t.Errorf("hints on: shown=%v pending=%v atoms=%q", shown, statErr, all)
				}
			} else {
				if shown || statErr == nil {
					t.Errorf("hints off: shown=%v pending=%v atoms=%q", shown, statErr, all)
				}
			}
			if !slices.Contains(all, "adopt:option=--json") {
				t.Errorf("uses must be recorded regardless of hints: %q", all)
			}
		})
	}
}
