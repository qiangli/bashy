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

func newTestInboxHinter(agent bool) (*inboxHinter, *bytes.Buffer) {
	var buf bytes.Buffer
	m := &memory{hosts: map[string]hostRecord{}, fails: map[string]failMark{}, hinted: map[string]bool{}}
	return &inboxHinter{agent: agent, mem: m, w: &buf}, &buf
}

// The one rule: reading the board through mb --history, a bus read, a bare
// ping, or the chat timeline points the agent at `bashy inbox`; every write /
// send / ICMP path is left alone.
func TestInboxReadSuggestMatches(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"mb --history", []string{"mb", "--history"}, true},
		{"mb --history --json", []string{"mb", "--history", "--json"}, true},
		{"mb plain read", []string{"mb"}, false},
		{"mb send", []string{"mb", "send", "steward", "hi"}, false},
		{"bus bare", []string{"bus"}, true},
		{"bus watch", []string{"bus", "watch", "--drain"}, true},
		{"bus pending", []string{"bus", "pending"}, true},
		{"bus publish", []string{"bus", "publish", "--topic", "b", "x"}, false},
		{"ping bare read", []string{"ping"}, true},
		{"ping host (ICMP)", []string{"ping", "dragon"}, false},
		{"ping send", []string{"ping", "steward", "hi"}, false},
		{"chat timeline", []string{"chat", "timeline"}, true},
		{"chat steer", []string{"chat", "steer", "1", "hi"}, false},
		{"unrelated", []string{"ls", "-l"}, false},
		{"empty", []string{}, false},
	}
	for _, c := range cases {
		if got := inboxReadSuggest(c.args) != ""; got != c.want {
			t.Errorf("%s: inboxReadSuggest matched=%v, want %v", c.name, got, c.want)
		}
	}
}

// The hint rides the existing bashy-hint-v1 engine on stderr; running with
// --json must not change that — the hint is its own JSON line and the command's
// stdout is never touched by the hinter.
func TestInboxHintAgentJSONParseable(t *testing.T) {
	h, buf := newTestInboxHinter(true)
	h.onAudit(interp.AuditEvent{Args: []string{"mb", "--history", "--json"}})
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected an inbox hint for mb --history")
	}
	var nl nudgeLine
	if err := json.Unmarshal([]byte(line), &nl); err != nil {
		t.Fatalf("hint is not a parseable JSON line: %v (%q)", err, line)
	}
	if nl.Schema != nudgeSchemaVersion || nl.Kind != "hint" {
		t.Errorf("unexpected hint envelope: %+v", nl)
	}
	if !strings.Contains(nl.Suggest, "bashy inbox") {
		t.Errorf("hint should name bashy inbox, got %q", nl.Suggest)
	}
	if nl.Off == "" {
		t.Error("hint must carry the off switch")
	}
}

func TestInboxHintRateLimitedOncePerSession(t *testing.T) {
	h, buf := newTestInboxHinter(true)
	for range 4 {
		h.onAudit(interp.AuditEvent{Args: []string{"bus", "watch"}})
	}
	if n := strings.Count(buf.String(), "\n"); n != 1 {
		t.Errorf("inbox hint fired %d times, want exactly 1 per session", n)
	}
}

func TestInboxHintHumanProse(t *testing.T) {
	h, buf := newTestInboxHinter(false)
	h.onAudit(interp.AuditEvent{Args: []string{"chat", "timeline"}})
	out := buf.String()
	if !strings.Contains(out, "bashy inbox") || strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("human-mode inbox hint malformed: %q", out)
	}
}
