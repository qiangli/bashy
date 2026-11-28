// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// Targeted tests for `bashy <tool>` (toolshortcut.go): name resolution
// against the fleet catalog, session-id derivation, argument parsing, and
// the non-TTY refusal.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeShortcutToolsRing points the fleet tools overlay at a scratch dir.
// Callers must resetToolShortcutCache after (deferred) so the cached front-door
// set cannot leak between tests sharing the process.
func writeShortcutToolsRing(t *testing.T, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("BASHY_TOOLS_PATH", dir)
	resetToolShortcutCache()
	t.Cleanup(resetToolShortcutCache)
}

const shortcutSteerToolYAML = `name: teststeer
kind: cli
display: Test Steer
aliases: [ts-alias]
cli:
  binary: teststeer-bin
  launch:
    exec: teststeer-bin -p {prompt}
    steer_exec: teststeer-bin
`

const shortcutPlainToolYAML = `name: testplain
kind: cli
display: Test Plain
cli:
  binary: testplain-bin
  launch:
    exec: testplain-bin -p {prompt}
`

// The acceptance name resolves against the real pinned catalog: `bashy
// codex` must find a registered steerable tool, and a name nobody registered
// must miss (so the front door keeps answering 127 for it).
func TestToolShortcutBaselineResolution(t *testing.T) {
	resetToolShortcutCache()
	t.Cleanup(resetToolShortcutCache)
	tool, ok := lookupSteerTool("codex")
	if !ok {
		t.Fatal("lookupSteerTool(codex) = false, want the registered steerable tool")
	}
	if tool.Name != "codex" {
		t.Errorf("canonical tool name = %q, want codex", tool.Name)
	}
	if _, ok := lookupSteerTool("definitely-not-a-tool-xyz"); ok {
		t.Error("lookupSteerTool(unknown) = true, want false")
	}
	if !isToolShortcutName("codex") {
		t.Error("isToolShortcutName(codex) = false, want true")
	}
	if isToolShortcutName("definitely-not-a-tool-xyz") {
		t.Error("isToolShortcutName(unknown) = true, want false")
	}
	if isToolShortcutName("") || isToolShortcutName("--help") || isToolShortcutName("a/b") {
		t.Error("isToolShortcutName must reject empty names, flags and paths")
	}
}

// The steer predicate is dynamic: a ring tool with steer_exec resolves, one
// with only a headless exec does not, and aliases resolve to the canonical
// tool (which owns the session id).
func TestToolShortcutSyntheticRing(t *testing.T) {
	writeShortcutToolsRing(t, map[string]string{
		"teststeer.yaml": shortcutSteerToolYAML,
		"testplain.yaml": shortcutPlainToolYAML,
	})
	tool, ok := lookupSteerTool("teststeer")
	if !ok {
		t.Fatal("lookupSteerTool(teststeer) = false, want true (steer_exec declared)")
	}
	if tool.Name != "teststeer" {
		t.Errorf("canonical name = %q, want teststeer", tool.Name)
	}
	if aliased, ok := lookupSteerTool("ts-alias"); !ok || aliased.Name != "teststeer" {
		t.Errorf("lookupSteerTool(ts-alias) = %+v, %v, want canonical teststeer", aliased, ok)
	}
	if _, ok := lookupSteerTool("testplain"); ok {
		t.Error("lookupSteerTool(testplain) = true, want false (headless only)")
	}
	if !isToolShortcutName("teststeer") || isToolShortcutName("testplain") {
		t.Error("front-door set must carry teststeer and not testplain")
	}
}

// The session id's base is the tool name (or tool:model), sanitized exactly
// like chat derives it — including the dot-mangling the operator hit
// (codex-gpt6.1-sol -> codex-gpt6-1-sol).
func TestToolShortcutSessionID(t *testing.T) {
	for agent, want := range map[string]string{
		"codex":            "codex",
		"agy":              "agy",
		"codex-gpt6.1-sol": "codex-gpt6-1-sol",
		"codex:gpt6":       "codex-gpt6",
	} {
		if got := toolShortcutSessionID(agent); got != want {
			t.Errorf("toolShortcutSessionID(%q) = %q, want %q", agent, got, want)
		}
	}
}

func TestToolShortcutAgentName(t *testing.T) {
	if got := toolShortcutAgentName("codex", ""); got != "codex" {
		t.Errorf("default agent = %q, want codex (vendor default model)", got)
	}
	if got := toolShortcutAgentName("codex", "gpt6"); got != "codex:gpt6" {
		t.Errorf("--model agent = %q, want codex:gpt6", got)
	}
}

func TestParseToolShortcutArgs(t *testing.T) {
	// Defaults: yolo on (operator decision), nothing else set.
	opts, err := parseToolShortcutArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.yolo || opts.model != "" || opts.help || opts.explain || opts.dryRun || opts.passthrough != nil || len(opts.prompt) != 0 {
		t.Errorf("defaults = %+v, want yolo on and nothing else", opts)
	}
	// Full flag shape.
	opts, err = parseToolShortcutArgs([]string{"--model", "gpt6", "--no-yolo", "do", "the thing"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.model != "gpt6" || opts.yolo {
		t.Errorf("--model/--no-yolo = %+v", opts)
	}
	if strings.Join(opts.prompt, " ") != "do the thing" {
		t.Errorf("prompt = %q", opts.prompt)
	}
	// --model= form and explicit --yolo.
	opts, err = parseToolShortcutArgs([]string{"--model=gpt6", "--yolo"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.model != "gpt6" || !opts.yolo {
		t.Errorf("--model=/--yolo = %+v", opts)
	}
	// -- ends option parsing: everything after is raw passthrough.
	opts, err = parseToolShortcutArgs([]string{"--model", "gpt6", "--", "exec", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.passthrough == nil || strings.Join(opts.passthrough, " ") != "exec --json" {
		t.Errorf("passthrough = %q", opts.passthrough)
	}
	if len(opts.prompt) != 0 {
		t.Errorf("prompt after -- must stay empty, got %q", opts.prompt)
	}
	// Errors.
	for _, args := range [][]string{
		{"--bogus"},
		{"--model"},
		{"--model="},
	} {
		if _, err := parseToolShortcutArgs(args); err == nil {
			t.Errorf("parse(%q) = nil error, want an error", args)
		}
	}
}

// A non-terminal stdin refuses with guidance: the TUI would exit at once,
// and the operator (often an agent's `!` shell) needs the way out spelled.
func TestCheckToolShortcutTTYRefusal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	err = checkToolShortcutTTY("codex", r)
	if err == nil {
		t.Fatal("pipe stdin: want refusal, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"codex", "not a TTY", "bashy chat --agent codex"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal %q lacks %q", msg, want)
		}
	}
	// A missing stdin is never a terminal session either — refuse, loudly.
	if err := checkToolShortcutTTY("codex", nil); err == nil {
		t.Error("nil stdin: want refusal, got nil")
	}
}

// --help shows the resolved model, binding and steer id.
func TestToolShortcutHelp(t *testing.T) {
	resetToolShortcutCache()
	t.Cleanup(resetToolShortcutCache)
	tool, ok := lookupSteerTool("codex")
	if !ok {
		t.Skip("codex not registered in this catalog")
	}
	var buf bytes.Buffer
	toolShortcutHelp(&buf, tool, toolShortcutOpts{yolo: true})
	out := buf.String()
	for _, want := range []string{
		"binding:  codex",
		"model:    (vendor default)",
		"session:  codex",
		"bashy chat steer codex",
		"bashy chat attach codex",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--help lacks %q:\n%s", want, out)
		}
	}
	// --model switches the binding, model and session id together.
	toolShortcutHelp(&buf, tool, toolShortcutOpts{yolo: true, model: "gpt6"})
	out = buf.String()
	for _, want := range []string{"binding:  codex:gpt6", "model:    gpt6", "session:  codex-gpt6-<8 hex>"} {
		if !strings.Contains(out, want) {
			t.Errorf("--help --model lacks %q:\n%s", want, out)
		}
	}
}

// --dry-run resolves the steer launch without starting anything.
func TestToolShortcutDryRun(t *testing.T) {
	writeShortcutToolsRing(t, map[string]string{
		"teststeer.yaml": shortcutSteerToolYAML,
	})
	var stdout, stderr bytes.Buffer
	if code := dispatchToolShortcut("teststeer", []string{"--dry-run"}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("--dry-run exit = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"session: teststeer-<8 hex>", "teststeer-bin", "bashy chat steer teststeer"} {
		if !strings.Contains(out, want) {
			t.Errorf("--dry-run lacks %q:\n%s", want, out)
		}
	}
}

// --help through the dispatcher (lookup + usage, no TTY needed).
func TestDispatchToolShortcutHelp(t *testing.T) {
	resetToolShortcutCache()
	t.Cleanup(resetToolShortcutCache)
	var stdout, stderr bytes.Buffer
	if code := dispatchToolShortcut("codex", []string{"--help"}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("--help exit = %d, stderr = %s", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, "session:  codex-<8 hex>") {
		t.Errorf("--help lacks the session id:\n%s", out)
	}
}

// The vendor passthrough runs the binary with forwarded stdio and exit
// status — here a shell exiting 3.
func TestRunToolPassthroughExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runToolPassthrough("/bin/sh", []string{"-c", "exit 3"}, nil, &stdout, &stderr); code != 3 {
		t.Errorf("passthrough exit = %d, want 3 (stderr = %s)", code, stderr.String())
	}
}

// The operator's own ring cannot swallow a shortcut name: `commands add`
// refuses it by holder.
func TestReservedCommandNameToolShortcut(t *testing.T) {
	writeShortcutToolsRing(t, map[string]string{
		"teststeer.yaml": shortcutSteerToolYAML,
	})
	holder, ok := reservedCommandName("teststeer")
	if !ok {
		t.Fatal("reservedCommandName(teststeer) = false, want refused (shortcut would be swallowed)")
	}
	if !strings.Contains(holder, "bashy teststeer") {
		t.Errorf("holder = %q, want the shortcut named", holder)
	}
	if _, ok := reservedCommandName("definitely-not-a-tool-xyz"); ok {
		t.Error("reservedCommandName(unknown) = true, want false")
	}
}
