package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
)

// The adapter entry builds alone (GOWORK=off) and links no other bashy or
// yoke command package; its help still carries the owner-accountability
// contract and the bashy-owned subcommands duplicated into this package.
func TestStandaloneHelpAndDeps(t *testing.T) {
	sprintWatchIsolate(t)
	var out, errBuf bytes.Buffer
	if code := runSprint([]string{"--help"}, &out, &errBuf); code != 0 {
		t.Fatalf("sprint --help exited %d: stderr=%s", code, errBuf.String())
	}
	help := out.String()
	for _, want := range []string{"OWNER ACCOUNTABILITY", "inbox-ack", "monitor", "await", "wait"} {
		if !strings.Contains(help, want) {
			t.Errorf("sprint --help lacks %q", want)
		}
	}
	adapter.AssertStandaloneDeps(t, "sprint")
}
