// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func newTestNotFoundHinter() *notFoundHinter {
	return &notFoundHinter{
		provided: []string{"cat", "grep", "ls", "awd", "git", "printf"},
		gnuCore:  map[string]bool{"gdate": true, "gstat": true},
		seen:     map[string]bool{},
	}
}

func TestNotFoundHintsEnabledGate(t *testing.T) {
	// Inert without the env.
	t.Setenv("BASHY_HINTS", "")
	t.Setenv("BASHY_AGENTIC", "")
	if notFoundHintsEnabled() {
		t.Error("must be inert when BASHY_AGENTIC is unset")
	}
	for _, v := range []string{"0", "off", "false", "no"} {
		t.Setenv("BASHY_AGENTIC", v)
		if notFoundHintsEnabled() {
			t.Errorf("BASHY_AGENTIC=%q must stay inert", v)
		}
	}
	for _, v := range []string{"1", "on", "true", "yes"} {
		t.Setenv("BASHY_AGENTIC", v)
		if !notFoundHintsEnabled() {
			t.Errorf("BASHY_AGENTIC=%q should enable the hint", v)
		}
	}
	// BASHY_HINTS=off is the shared silencer even with BASHY_AGENTIC on.
	t.Setenv("BASHY_AGENTIC", "1")
	for _, v := range []string{"0", "off", "false", "no"} {
		t.Setenv("BASHY_HINTS", v)
		if notFoundHintsEnabled() {
			t.Errorf("BASHY_HINTS=%q must silence the hint", v)
		}
	}
}

func TestNearestProvidedDidYouMean(t *testing.T) {
	provided := []string{"cat", "grep", "ls", "awd", "git", "printf", "mkdir"}
	cases := []struct {
		want, nearest string
	}{
		{"gerp", "grep"},    // transposition, distance 2
		{"grpe", "grep"},    // transposition
		{"mkdr", "mkdir"},   // one deletion
		{"pintf", "printf"}, // one insertion
		{"grep", ""},        // exact match is not a suggestion
		{"zzzzzzz", ""},     // nothing close
		{"xy", ""},          // too short, no close match
	}
	for _, c := range cases {
		if got := nearestProvided(c.want, provided); got != c.nearest {
			t.Errorf("nearestProvided(%q) = %q, want %q", c.want, got, c.nearest)
		}
	}
}

func TestNotFoundClassify(t *testing.T) {
	h := newTestNotFoundHinter()

	// A typo of a provided name: not_found tier, a nearest suggestion, no door.
	nf := h.classify("gre")
	if nf.tier != "not_found" {
		t.Errorf("tier = %q, want not_found", nf.tier)
	}
	if nf.nearest != "grep" {
		t.Errorf("nearest = %q, want grep", nf.nearest)
	}
	if nf.install != "" {
		t.Errorf("install = %q, want empty (no door)", nf.install)
	}

	// A GNU coreutils name carries the container install door.
	nf = h.classify("gdate")
	if nf.install != "gnu-coreutils-container" {
		t.Errorf("install = %q, want gnu-coreutils-container", nf.install)
	}

	// Something with no neighbour and no door.
	nf = h.classify("zzzxyzzz")
	if nf.nearest != "" || nf.install != "" {
		t.Errorf("unexpected hint for unknown name: %+v", nf)
	}
}

func TestNotFoundEmitJSONShape(t *testing.T) {
	h := newTestNotFoundHinter()
	var buf bytes.Buffer
	if !h.emit(&buf, "gdate") {
		t.Fatal("emit should have fired")
	}
	line := strings.TrimSpace(buf.String())
	var nl notFoundLine
	if err := json.Unmarshal([]byte(line), &nl); err != nil {
		t.Fatalf("hint not valid JSON: %v (%q)", err, line)
	}
	if nl.Schema != nudgeSchemaVersion {
		t.Errorf("schema = %q, want %q", nl.Schema, nudgeSchemaVersion)
	}
	if nl.Kind != "command-not-found" {
		t.Errorf("kind = %q, want command-not-found", nl.Kind)
	}
	if nl.Tool != "gdate" || nl.Tier != "not_found" {
		t.Errorf("unexpected tool/tier: %+v", nl)
	}
	if nl.Install != "gnu-coreutils-container" {
		t.Errorf("install = %q, want gnu-coreutils-container", nl.Install)
	}
	if nl.Off == "" || nl.Suggest == "" {
		t.Errorf("hint must carry suggest and off-switch: %+v", nl)
	}
}

func TestNotFoundEmitRateLimitedOncePerName(t *testing.T) {
	h := newTestNotFoundHinter()
	var buf bytes.Buffer
	for range 5 {
		h.emit(&buf, "gdate")
	}
	if got := strings.Count(buf.String(), "\n"); got != 1 {
		t.Errorf("emitted %d lines, want exactly 1 (once per name)", got)
	}
}

// runNotFound runs script through a runner wired with the not-found middleware
// over the default (real-exec) handler, and returns stdout+stderr. The runner
// inherits the process environment and cwd, so the script itself drives PATH and
// cwd — which is the whole point: the handler must resolve against the shell's
// live context, not the process one.
func runNotFound(t *testing.T, h *notFoundHinter, script string) (stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	r, err := interp.New(interp.StdIO(nil, &out, &errOut), interp.ExecHandlers(notFoundHintHandler(h)))
	if err != nil {
		t.Fatal(err)
	}
	prog, perr := syntax.NewParser().Parse(strings.NewReader(script), "")
	if perr != nil {
		t.Fatal(perr)
	}
	_ = r.Run(context.Background(), prog)
	return out.String(), errOut.String()
}

// writeExit127 drops an executable shebang script that exits 127 and returns its
// directory; it is a REAL command whose 127 is its own, not a PATH miss.
func writeExit127(t *testing.T, dir, name string) {
	t.Helper()
	bin := filepath.Join(dir, name)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 127\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// A real command reachable only on a PATH the SCRIPT assigns (never on the
// process PATH) must not be reported as not-found when it exits 127. The old
// os/exec.LookPath against the process environment could not see the shell-local
// PATH and wrongly flagged it; resolution now uses the handler context.
func TestNotFoundResolvesShellLocalPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a directly executable shebang script")
	}
	h := newTestNotFoundHinter()
	dir := t.TempDir()
	writeExit127(t, dir, "shelllocaltool")
	// PATH is set INSIDE the script; the process PATH never contains dir.
	script := "export PATH=" + dir + "\nshelllocaltool ; echo status=$?\n"
	out, errOut := runNotFound(t, h, script)
	if !strings.Contains(out, "status=127") {
		t.Fatalf("expected the shell-local command to run and exit 127, stdout=%q", out)
	}
	if strings.Contains(errOut, "command-not-found") {
		t.Errorf("a real command on the shell-local PATH was wrongly flagged:\n%s", errOut)
	}
}

// A relative PATH entry read after a `cd` must resolve against the shell's cwd,
// exactly as the command does. os/exec.LookPath would resolve it against the
// process cwd and miss it, producing a false hint.
func TestNotFoundResolvesRelativePATHAfterCd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a directly executable shebang script")
	}
	h := newTestNotFoundHinter()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExit127(t, sub, "reltool")
	// cd into dir, put the RELATIVE entry "sub" on PATH, then run reltool.
	script := "cd " + dir + "\nexport PATH=sub\nreltool ; echo status=$?\n"
	out, errOut := runNotFound(t, h, script)
	if !strings.Contains(out, "status=127") {
		t.Fatalf("expected reltool to run and exit 127, stdout=%q", out)
	}
	if strings.Contains(errOut, "command-not-found") {
		t.Errorf("a command on a relative shell-local PATH was wrongly flagged:\n%s", errOut)
	}
}

// The mirror case: a genuine shell-local miss must still be hinted even when the
// host PATH would satisfy the name. Here the script narrows PATH so the command
// is unreachable; the old process-PATH lookup would have found it and silently
// suppressed the hint.
func TestNotFoundHintsShellLocalMissMaskedByHostPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a directly executable shebang script")
	}
	h := newTestNotFoundHinter()
	dir := t.TempDir()
	writeExit127(t, dir, "onhostonly")
	// Put dir on the PROCESS PATH so os/exec.LookPath would resolve the name...
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// ...but a per-command assignment narrows PATH so the SHELL cannot: a real
	// not-found that must be hinted.
	script := "PATH=/no/such/dir onhostonly ; echo status=$?\n"
	out, errOut := runNotFound(t, h, script)
	if !strings.Contains(out, "status=127") {
		t.Fatalf("expected a shell-local miss to exit 127, stdout=%q", out)
	}
	if !strings.Contains(errOut, "command-not-found") {
		t.Errorf("a genuine shell-local miss was not hinted (host PATH masked it):\n%s", errOut)
	}
}

// End-to-end: a genuinely absent command run through the middleware still exits
// 127 with Bash's own error text, and the structured hint is appended AFTER it.
func TestNotFoundHandlerAppendsHintPreservingBashOutput(t *testing.T) {
	h := newTestNotFoundHinter()
	run := func(withHint bool) (stdout, stderr string) {
		var out, errOut bytes.Buffer
		var r *interp.Runner
		var err error
		if withHint {
			r, err = interp.New(interp.StdIO(nil, &out, &errOut), interp.ExecHandlers(notFoundHintHandler(h)))
		} else {
			r, err = interp.New(interp.StdIO(nil, &out, &errOut))
		}
		if err != nil {
			t.Fatal(err)
		}
		script := "no-such-command-zzz-42 ; echo status=$?\n"
		prog, perr := syntax.NewParser().Parse(strings.NewReader(script), "")
		if perr != nil {
			t.Fatal(perr)
		}
		_ = r.Run(context.Background(), prog)
		return out.String(), errOut.String()
	}

	baseOut, baseErr := run(false)
	if !strings.Contains(baseOut, "status=127") {
		t.Fatalf("expected exit 127 for an absent command, stdout=%q", baseOut)
	}

	hintOut, hintErr := run(true)
	if hintOut != baseOut {
		t.Errorf("stdout changed by the hint: base=%q hint=%q", baseOut, hintOut)
	}
	// Bash's error text is unchanged: the hinted stderr starts with the exact
	// baseline, and only adds the JSON line.
	if !strings.HasPrefix(hintErr, baseErr) {
		t.Errorf("bash error text changed:\n base=%q\n hint=%q", baseErr, hintErr)
	}
	extra := strings.TrimSpace(strings.TrimPrefix(hintErr, baseErr))
	var nl notFoundLine
	if err := json.Unmarshal([]byte(extra), &nl); err != nil {
		t.Fatalf("appended hint not valid JSON: %v (%q)", err, extra)
	}
	if nl.Kind != "command-not-found" || nl.Tool != "no-such-command-zzz-42" {
		t.Errorf("unexpected hint: %+v", nl)
	}
}

// A real command that exits 127 must NOT be annotated as command-not-found.
func TestNotFoundHandlerSkipsRealCommandExiting127(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a directly executable shebang script")
	}
	h := newTestNotFoundHinter()
	dir := t.TempDir()
	bin := filepath.Join(dir, "exits127")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 127\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out, errOut bytes.Buffer
	r, err := interp.New(interp.StdIO(nil, &out, &errOut), interp.ExecHandlers(notFoundHintHandler(h)))
	if err != nil {
		t.Fatal(err)
	}
	prog, perr := syntax.NewParser().Parse(strings.NewReader("exits127 ; echo status=$?\n"), "")
	if perr != nil {
		t.Fatal(perr)
	}
	_ = r.Run(context.Background(), prog)
	if !strings.Contains(out.String(), "status=127") {
		t.Fatalf("expected the real command to exit 127, stdout=%q", out.String())
	}
	if strings.Contains(errOut.String(), "command-not-found") {
		t.Errorf("a real command exiting 127 was wrongly annotated:\n%s", errOut.String())
	}
}

// The hint is wired only in the non-posix agentic path: a --posix shell (and so
// the cmd/bash drop-in) must never emit it, even with the agentic env on.
func TestNotFoundHintInertUnderPosix(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1")
	t.Setenv("BASHY_HINTS", "1")
	var out, errOut bytes.Buffer
	r, err := interp.New(interp.Env(nil), interp.Params("-o", "posix"))
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range wireExec(nil, true, os.Environ(), nil, &out, &errOut, false) {
		if err := opt(r); err != nil {
			t.Fatal(err)
		}
	}
	prog, perr := syntax.NewParser().Parse(strings.NewReader("no-such-command-posix-zz ; echo status=$?\n"), "")
	if perr != nil {
		t.Fatal(perr)
	}
	_ = r.Run(context.Background(), prog)
	if !strings.Contains(out.String(), "status=127") {
		t.Fatalf("expected the absent command to exit 127, stdout=%q", out.String())
	}
	if strings.Contains(errOut.String(), "command-not-found") {
		t.Errorf("the command-not-found hint fired under --posix:\n%s", errOut.String())
	}
}
