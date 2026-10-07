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

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"grep", "grep", 0},
		{"grep", "gerp", 2},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
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

func TestNotFoundIsNotFound(t *testing.T) {
	h := newTestNotFoundHinter()
	// A clearly-absent bare name is not found.
	if !h.isNotFound("definitely-not-a-real-command-xyz-9z") {
		t.Error("absent command should be reported not found")
	}
	// A real, on-PATH command is NOT reported not found: a real command that
	// exits 127 must never be misclassified.
	dir := t.TempDir()
	realCmd := "realcmd127"
	ext := ""
	body := "#!/bin/sh\nexit 127\n"
	if runtime.GOOS == "windows" {
		ext = ".bat"
		body = "@exit /b 127\r\n"
	}
	bin := filepath.Join(dir, realCmd+ext)
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	lookupName := realCmd
	if runtime.GOOS == "windows" {
		lookupName = realCmd + ext
	}
	if h.isNotFound(lookupName) {
		t.Errorf("%q is on PATH and must not be reported not found", lookupName)
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
