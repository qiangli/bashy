// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build e2e

// Process-level evidence for the @confirm → shared approval queue bridge: the
// real bashy binary yields, `bashy approve` lists and settles the persisted
// record, and a fresh process resumes with the same token exactly once.
package agentos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const confirmE2EScript = `@confirm()
function clean() {
	rm -rf "$1/victim"
	echo "done"
}
clean "$@"
echo "status $?"
`

type e2eApproval struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Token  string `json:"token"`
	Resume string `json:"resume"`
}

func TestE2EConfirmYieldApproveResume(t *testing.T) {
	bin := bashyBinary(t)
	work := t.TempDir()
	script := filepath.Join(work, "clean.bsh")
	if err := os.WriteFile(script, []byte(confirmE2EScript), 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{"BASHY_ASK_DIR=" + filepath.Join(work, "ask"), "BASHY_SESSION_CLAIM=e2e-live", "BASHY_SPRINT=418"}
	victim := filepath.Join(work, "victim")
	mkVictim := func() {
		if err := os.MkdirAll(victim, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	run := func(env []string, args ...string) (string, int) {
		t.Helper()
		out, _, code := runBashyStdEnv(bin, env, args...)
		return out, code
	}
	pending := func() []e2eApproval {
		t.Helper()
		out, _, _ := runBashyStdEnv(bin, env, "approve", "list", "--json")
		var v struct {
			Approvals []e2eApproval `json:"approvals"`
		}
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatalf("approve list --json: %v\n%s", err, out)
		}
		return v.Approvals
	}
	resume := func(env []string, flag string) string {
		t.Helper()
		out, _ := runBashyEnv(bin, env, "--bashsharp", script, flag, work)
		return out
	}

	mkVictim()
	out, _ := run(env, "--bashsharp", script, work)
	if !strings.Contains(out, "status 6") {
		t.Fatalf("first run did not yield: %q", out)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("yielded rm ran")
	}
	list := pending()
	if len(list) != 1 || list[0].State != "pending" {
		t.Fatalf("approve list after yield = %+v", list)
	}
	tok, id := list[0].Token, list[0].ID

	// Allow it; a different session and a different argv never redeem it.
	if out, code := run(env, "approve", "allow", id[:8], "--token", tok); code != 0 || !strings.Contains(out, "--confirm="+tok+":yes") {
		t.Fatalf("approve allow: %q (exit %d)", out, code)
	}
	otherSession := append(append([]string(nil), env...), "BASHY_SESSION_CLAIM=e2e-other")
	if out := resume(otherSession, "--confirm="+tok+":yes"); !strings.Contains(out, "status 6") {
		t.Fatalf("another session redeemed the allow: %q", out)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("another session ran the operation")
	}

	// Same session resumes once.
	if out := resume(env, "--confirm="+tok+":yes"); !strings.Contains(out, "status 0") {
		t.Fatalf("resume: %q", out)
	}
	if _, err := os.Stat(victim); err == nil {
		t.Fatal("resume did not run the operation")
	}

	// The same token again is spent: it yields and nothing runs.
	mkVictim()
	if out := resume(env, "--confirm="+tok+":yes"); !strings.Contains(out, "status 6") {
		t.Fatalf("duplicate resume: %q", out)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("a spent allow ran the operation twice")
	}

	// Deny the new pending approval (the duplicate's) and resume: refused, repeatedly.
	var denyID string
	for _, a := range pending() {
		if a.State == "pending" && a.ID != id {
			denyID = a.ID
		}
	}
	if denyID == "" {
		t.Fatal("the duplicate resume raised no new approval")
	}
	// The yielded-by-other-session approval is also pending under the same token;
	// deny them all so only the live session's decision is under test.
	for _, a := range pending() {
		if a.State == "pending" {
			if out, code := run(env, "approve", "deny", a.ID[:8]); code != 0 || !strings.Contains(out, "--confirm="+tok+":no") {
				t.Fatalf("approve deny: %q (exit %d)", out, code)
			}
		}
	}
	for i := 0; i < 2; i++ {
		out := resume(env, "--confirm="+tok+":yes")
		if !strings.Contains(out, "status 0") || !strings.Contains(out, "declined "+tok) {
			t.Fatalf("denied resume %d: %q", i, out)
		}
		if _, err := os.Stat(victim); err != nil {
			t.Fatal("a denied operation ran")
		}
	}
}
