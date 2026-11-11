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
