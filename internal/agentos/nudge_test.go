// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
)

func newTestNudger(t *testing.T, agent bool) (*nudger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	m := &memory{hosts: map[string]hostRecord{}, fails: map[string]failMark{}, hinted: map[string]bool{}}
	return &nudger{agent: agent, mem: m, w: &buf}, &buf
}

func TestNudgeCdHintAgentMode(t *testing.T) {
	n, buf := newTestNudger(t, true)
	n.onAudit(interp.AuditEvent{IsBuiltin: true, Args: []string{"cd", "/tmp"}})
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected a hint for cd")
	}
	var nl nudgeLine
	if err := json.Unmarshal([]byte(line), &nl); err != nil {
		t.Fatalf("hint not valid JSON: %v (%q)", err, line)
	}
	if nl.Schema != nudgeSchemaVersion || nl.Kind != "hint" || nl.Tool != "cd" {
		t.Errorf("unexpected hint shape: %+v", nl)
	}
	if !strings.Contains(nl.Suggest, "awd") {
		t.Errorf("cd hint should point at awd: %q", nl.Suggest)
	}
	if nl.Off == "" {
		t.Error("hint must include the off-switch")
	}
}

func TestNudgeRateLimitedOncePerSession(t *testing.T) {
	n, buf := newTestNudger(t, true)
	for range 5 {
		n.onAudit(interp.AuditEvent{IsBuiltin: true, Args: []string{"cd", "/x"}})
	}
	if got := strings.Count(buf.String(), "\n"); got != 1 {
		t.Errorf("cd nudged %d times, want exactly 1 (once per session)", got)
	}
}

func TestNudgeIgnoresNonWatchedAndExternals(t *testing.T) {
	n, buf := newTestNudger(t, true)
	n.onAudit(interp.AuditEvent{IsBuiltin: true, Args: []string{"echo", "hi"}}) // not watched
	n.onAudit(interp.AuditEvent{IsBuiltin: false, Args: []string{"cd"}})        // external, not a builtin
	n.onAudit(interp.AuditEvent{IsBuiltin: true, Args: []string{}})             // no args
	if buf.String() != "" {
		t.Errorf("expected no hints, got %q", buf.String())
	}
}

func TestNudgeHumanModeProse(t *testing.T) {
	n, buf := newTestNudger(t, false)
	n.onAudit(interp.AuditEvent{IsBuiltin: true, Args: []string{"pushd", "/d"}})
	out := buf.String()
	if !strings.Contains(out, "bashy hint") || !strings.Contains(out, "awd") {
		t.Errorf("human prose hint malformed: %q", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Error("human mode must not emit JSON")
	}
}

// The routing-rule cases (grep -r → hint, --agentic/--json suppress, find → ast
// symbols) now live in coreutils/pkg/nudge's TestSuggestBuiltinAndRouting, the
// single source of truth. This file keeps the bashy-side audit→emit integration.

func TestRoutingHintEmittedViaAudit(t *testing.T) {
	n, buf := newTestNudger(t, true)
	n.onAudit(interp.AuditEvent{IsBuiltin: false, Args: []string{"grep", "-r", "x", "."}})
	if !strings.Contains(buf.String(), "ast refs") {
		t.Errorf("expected a grep routing hint mentioning `ast refs`, got %q", buf.String())
	}
	// rate-limited once per session
	n.onAudit(interp.AuditEvent{IsBuiltin: false, Args: []string{"grep", "-r", "y", "."}})
	if strings.Count(buf.String(), "\n") != 1 {
		t.Error("grep routing hint should fire once per session")
	}
}

func TestHintsEnabledControl(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	// BASHY_AGENTIC master kill beats everything.
	t.Setenv("BASHY_AGENTIC", "0")
	t.Setenv("BASHY_HINTS", "on")
	if hintsEnabled() {
		t.Error("BASHY_AGENTIC=0 must disable hints even with BASHY_HINTS=on")
	}
	t.Setenv("BASHY_AGENTIC", "")
	for _, v := range []string{"0", "off", "false", "no"} {
		t.Setenv("BASHY_HINTS", v)
		if hintsEnabled() {
			t.Errorf("BASHY_HINTS=%q should disable hints", v)
		}
	}
	for _, v := range []string{"1", "on", "true", "yes"} {
		t.Setenv("BASHY_HINTS", v)
		if !hintsEnabled() {
			t.Errorf("BASHY_HINTS=%q should enable hints", v)
		}
	}
}
