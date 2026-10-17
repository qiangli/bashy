package agentos

import (
	"bytes"
	"strings"
	"testing"
)

func TestChatDryRunUnroutableToolExitsZero(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	var out, errb bytes.Buffer
	code := runChatCmd([]string{"--tool", "codex", "-i", "--context", "prior context", "--dry-run"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit=%d want 0; out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "not routable") {
		t.Fatalf("output must name the unroutable reason: %q", out.String())
	}
}

func TestChatDryRunMalformedArgvStillFails(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	var out, errb bytes.Buffer
	if code := runChatCmd([]string{"--dry-run", "--no-such-flag"}, &out, &errb); code == 0 {
		t.Fatalf("unknown flag must exit non-zero")
	}
	if code := runChatCmd([]string{"--dry-run", "--band", "999"}, &out, &errb); code == 0 {
		t.Fatalf("out-of-range band must exit non-zero")
	}
}

func TestChatUnroutableWithoutDryRunStillFails(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	var out, errb bytes.Buffer
	if code := runChatCmd([]string{"--tool", "codex", "hello"}, &out, &errb); code == 0 {
		t.Fatalf("non-dry-run unroutable must exit non-zero")
	}
}
