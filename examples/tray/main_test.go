package main

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPollStatus(t *testing.T) {
	tests := []struct {
		name, presence, detail string
		detailErr              bool
		level                  string
	}{
		{"off", `{"running":false}`, ``, false, "off"},
		{"paired", `{"running":true}`, `{"state":"active (paired as alice)","status":{"configured":true}}`, false, "on"},
		{"identity only", `{"running":true}`, `{"state":"paired once as host (identity only)","status":{"configured":true}}`, false, "partial"},
		{"unpaired", `{"running":true}`, `{"state":"dormant (unpaired)","status":{"configured":false}}`, false, "partial"},
		{"status unavailable", `{"running":true}`, ``, true, "partial"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
				calls++
				if len(args) == 2 && args[1] == "--local-presence" {
					return []byte(tc.presence), nil
				}
				if tc.detailErr {
					return nil, errors.New("unavailable")
				}
				return []byte(tc.detail), nil
			}
			s := pollStatus(context.Background(), run)
			if got := s.level(); got != tc.level {
				t.Fatalf("level %s, want %s", got, tc.level)
			}
			want := 2
			if tc.level == "off" {
				want = 1
			}
			if calls != want {
				t.Fatalf("calls %d, want %d", calls, want)
			}
		})
	}
}
func TestConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tray.yaml")
	items, err := loadConfig(p)
	if err != nil || len(items) != 0 {
		t.Fatalf("missing config: %v %v", items, err)
	}
	data := []byte("items:\n  - label: Help\n    url: https://example.org/help\n")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	items, err = loadConfig(p)
	if err != nil || len(items) != 1 || items[0].Label != "Help" {
		t.Fatalf("config: %v %v", items, err)
	}
	for _, bad := range []string{"items:\n  - label: X\n    url: file:///tmp/x\n", "items:\n  - label: ''\n    url: https://example.org\n", "unknown: value\n"} {
		if err := os.WriteFile(p, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig(p); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
func TestAdminURL(t *testing.T) {
	t.Setenv("OUTPOST_ADMIN_ADDR", "127.0.0.1:17778")
	u, e := adminURL()
	if e != nil || u != "http://127.0.0.1:17778" {
		t.Fatalf("url %q: %v", u, e)
	}
	t.Setenv("OUTPOST_ADMIN_ADDR", "https://example.org")
	if _, e := adminURL(); e == nil {
		t.Fatal("accepted scheme in address")
	}
}
func TestIcon(t *testing.T) {
	for _, level := range []string{"on", "partial", "off"} {
		b := icon(level)
		if len(b) < 22 {
			t.Fatal("short icon")
		}
		if strings.HasPrefix(string(b), "\x00\x00\x01\x00") {
			if got := binary.LittleEndian.Uint32(b[18:22]); got != 22 {
				t.Fatalf("ico offset %d", got)
			}
		}
	}
}
