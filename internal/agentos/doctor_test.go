package agentos

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSiblingPins(t *testing.T) {
	pins := parseSiblingPins(`
# comment
sh=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
coreutils = bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
bad line
=missing-name
readline=
`)
	if pins["sh"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("sh pin missing: %#v", pins)
	}
	if pins["coreutils"] != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("coreutils pin missing: %#v", pins)
	}
	if _, ok := pins["readline"]; ok {
		t.Fatalf("empty sha should be ignored: %#v", pins)
	}
}

func TestFindBashySourceRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bashy")
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/qiangli/bashy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := findBashySourceRoot(nested)
	if !ok {
		t.Fatal("source root not found")
	}
	if got != root {
		t.Fatalf("source root = %q, want %q", got, root)
	}
}

func TestSiblingLayoutChecksWarnOnMissingSiblings(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "bashy")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sibling-pins"), []byte("sh=abc\ncoreutils=def\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sh", "go.mod"), []byte("module mvdan.cc/sh/v3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var checks []doctorCheck
	addSiblingLayoutChecks(&checks, root)
	var sawPins, sawMissing bool
	for _, c := range checks {
		if c.Name == "sibling pins" && c.Status == "ok" && strings.Contains(c.Detail, "coreutils") && strings.Contains(c.Detail, "sh") {
			sawPins = true
		}
		if c.Name == "sibling repos" && c.Status == "warn" && strings.Contains(c.Detail, "coreutils") {
			sawMissing = true
		}
	}
	if !sawPins || !sawMissing {
		t.Fatalf("unexpected sibling checks: %#v", checks)
	}
}

func TestCollectSelfChecksIncludesBootstrapSurface(t *testing.T) {
	checks := collectSelfChecks()
	need := map[string]bool{
		"bashy binary":   false,
		"embedded git":   false,
		"dag runner":     false,
		"managed go":     false,
		"release target": false,
		"bin cache":      false,
	}
	for _, c := range checks {
		if _, ok := need[c.Name]; ok {
			need[c.Name] = true
		}
	}
	for name, ok := range need {
		if !ok {
			t.Fatalf("collectSelfChecks missing %q in %#v", name, checks)
		}
	}
}

func TestDispatchDoctorJSONIncludesSelfChecks(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	prevStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = prevStdout }()
	code := dispatchDoctor([]string{"--json"})
	_ = w.Close()
	os.Stdout = prevStdout
	if code != 0 {
		t.Fatalf("dispatchDoctor = %d", code)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		SchemaVersion string        `json:"schema_version"`
		Checks        []doctorCheck `json:"checks"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("doctor JSON invalid: %v\n%s", err, data)
	}
	if payload.SchemaVersion != doctorSchemaVersion {
		t.Fatalf("schema = %q", payload.SchemaVersion)
	}
	var sawGit bool
	for _, c := range payload.Checks {
		if c.Name == "embedded git" && c.Status == "ok" {
			sawGit = true
		}
	}
	if !sawGit {
		t.Fatalf("doctor JSON missing embedded git check: %#v", payload.Checks)
	}
}
