package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
)

// The adapter entry builds alone (GOWORK=off) and links no other bashy or
// yoke command package, and its help renders through the adapter runner.
func TestStandaloneHelpAndDeps(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := runInbox([]string{"--help"}, &out, &errBuf); code != 0 {
		t.Fatalf("inbox --help exited %d: stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "bashy inbox") {
		t.Fatalf("inbox --help does not describe the inbox:\n%s", out.String())
	}
	adapter.AssertStandaloneDeps(t, "inbox")
}

// Usage errors keep the in-process contract: reported once with the
// "bashy inbox:" prefix and a non-zero exit.
func TestUsageErrorReportedWithPrefix(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := runInbox([]string{"--wait", "-1s"}, &out, &errBuf)
	if code == 0 {
		t.Fatal("negative --wait must fail")
	}
	if got := errBuf.String(); strings.Count(got, "bashy inbox: ") != 1 || !strings.Contains(got, "--wait must not be negative") {
		t.Fatalf("stderr = %q", got)
	}
}
