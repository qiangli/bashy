package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCLIRespondentAnswer(t *testing.T) {
	var preview bytes.Buffer
	if code := runCLI([]string{"answer", "answer-cli", "--dry-run", "--json"}, &preview); code != 0 || !strings.Contains(preview.String(), `"dry_run":true`) { t.Fatalf("answer preview: %d %s", code, preview.String()) }
	base := startServer(t)
	addr := strings.TrimPrefix(base, "http://")
	id, key := newForm(t, base, `{"action":"add","id":"answer-cli","title":"Question","definition":[{"id":"name","kind":"text","label":"Name"}]}`)
	var out bytes.Buffer
	if code := runCLI([]string{"answer", id, "--addr", addr, "--answer", "name=Ada", "--complete", "--json"}, &out); code != 0 {
		t.Fatalf("answer: %d %s", code, out.String())
	}
	var v map[string]any
	if err := json.Unmarshal(out.Bytes(), &v); err != nil || v["status"] != "answered" {
		t.Fatalf("answer: %s %v", out.String(), err)
	}
	out.Reset()
	if code := runCLI([]string{"show", id, "--addr", addr, "--json"}, &out); code != 1 || !strings.Contains(out.String(), "Join this form") {
		t.Fatalf("denied: %d %s", code, out.String())
	}
	out.Reset()
	if code := runCLI([]string{"show", id, "--addr", addr, "--key", key, "--json"}, &out); code != 0 || !strings.Contains(out.String(), "Ada") {
		t.Fatalf("owner show: %d %s", code, out.String())
	}
	out.Reset()
	if code := runCLI([]string{"answer", "missing", "--addr", addr, "--answer", "name=Ada", "--json"}, &out); code != 1 || !strings.Contains(out.String(), "Form not found") {
		t.Fatalf("missing form denied: %d %s", code, out.String())
	}
}

func TestCLIRespondentJoin(t *testing.T) {
	var preview bytes.Buffer
	if code := runCLI([]string{"join", "join-cli", "--invite", "tok123", "--dry-run", "--json"}, &preview); code != 0 || !strings.Contains(preview.String(), `"dry_run":true`) || !strings.Contains(preview.String(), `"verb":"join"`) {
		t.Fatalf("join preview: %d %s", code, preview.String())
	}
	base := startServer(t)
	addr := strings.TrimPrefix(base, "http://")
	id, ownerKey := newForm(t, base, `{"action":"add","id":"join-test","title":"Join Form","definition":[{"id":"q","kind":"text","label":"Q"}]}`)
	var shareOut bytes.Buffer
	if code := runCLI([]string{"share", id, "--addr", addr, "--key", ownerKey, "--json"}, &shareOut); code != 0 {
		t.Fatalf("share: %d %s", code, shareOut.String())
	}
	var shareObj map[string]any
	if err := json.Unmarshal(shareOut.Bytes(), &shareObj); err != nil || shareObj["invite"] == nil {
		t.Fatalf("share response: %s %v", shareOut.String(), err)
	}
	invite := shareObj["invite"].(string)

	// Valid join via --invite
	var joinOut bytes.Buffer
	participant := "p-" + id
	if code := runCLI([]string{"join", id, "--addr", addr, "--invite", invite, "--participant", participant, "--json"}, &joinOut); code != 0 {
		t.Fatalf("join: %d %s", code, joinOut.String())
	}
	var joinObj map[string]any
	if err := json.Unmarshal(joinOut.Bytes(), &joinObj); err != nil || joinObj["id"] != id {
		t.Fatalf("join response: %s %v", joinOut.String(), err)
	}

	// Reused invite is denied (403)
	joinOut.Reset()
	if code := runCLI([]string{"join", id, "--addr", addr, "--invite", invite, "--json"}, &joinOut); code != 1 || !strings.Contains(joinOut.String(), "invite is invalid") {
		t.Fatalf("reused invite denied: %d %s", code, joinOut.String())
	}

	// Missing form (404)
	joinOut.Reset()
	if code := runCLI([]string{"join", "nonexistent-form", "--addr", addr, "--invite", "someinvite", "--json"}, &joinOut); code != 1 || !strings.Contains(joinOut.String(), "Form not found") {
		t.Fatalf("missing form denied: %d %s", code, joinOut.String())
	}

	// Missing invite argument (exit 2)
	joinOut.Reset()
	if code := runCLI([]string{"join", id, "--addr", addr}, &joinOut); code != 2 || !strings.Contains(joinOut.String(), "needs --invite") {
		t.Fatalf("missing invite code: %d %s", code, joinOut.String())
	}
}
