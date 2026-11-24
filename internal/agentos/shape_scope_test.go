package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/yoke/pkg/shape"
	"github.com/qiangli/yoke/pkg/shape/appserver"
)

func TestShapeScopeCLI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shaped.json")
	original := []byte(`{"properties":["title"],"genie":{"hidden":true}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	oldLookup, oldActor := shapeAppLookup, shapeAuthoredActor
	t.Cleanup(func() { shapeAppLookup, shapeAuthoredActor = oldLookup, oldActor })
	shapeAppLookup = func(string) (shape.App, error) {
		return shape.App{Name: "site", SourceDir: dir, Owner: "owner@example.com"}, nil
	}
	shapeAuthoredActor = func(string) (string, error) { return "owner-agent", nil }
	t.Setenv("BASHY_PRINCIPAL", "owner@example.com")
	run := func(args ...string) (appserver.Scopes, error) {
		t.Helper()
		cmd := bashyShapeCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(append([]string{"scope"}, append(args, "--app", "site", "--json")...))
		err := cmd.Execute()
		var result struct {
			App    string           `json:"app"`
			Scopes appserver.Scopes `json:"scopes"`
		}
		if err == nil {
			if e := json.Unmarshal(out.Bytes(), &result); e != nil || result.App != "site" {
				t.Fatalf("invalid JSON result %s: %v", out.String(), e)
			}
		}
		return result.Scopes, err
	}
	if scopes, err := run("ls"); err != nil || !scopes.Empty() {
		t.Fatalf("ls: %+v %v", scopes, err)
	}
	if scopes, err := run("set", "shape", "--circle", "company", "--group", "support", "--email", "member@example.com"); err != nil || !scopes.AdmitsMember("shape", "company", []string{"support"}, "") || scopes.AdmitsMember("shape", "contractor", []string{"support"}, "") {
		t.Fatalf("set: %+v %v", scopes, err)
	}
	if _, err := run("set", "edit", "--email", "editor@example.com"); err != nil {
		t.Fatal(err)
	}
	if scopes, err := run("ls", "edit"); err != nil || scopes.Edit == nil || scopes.Shape != nil {
		t.Fatalf("filtered ls: %+v %v", scopes, err)
	}
	if scopes, err := run("rm", "shape"); err != nil || scopes.Shape != nil || scopes.Edit == nil {
		t.Fatalf("rm: %+v %v", scopes, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := appserver.Parse(data)
	if err != nil || len(manifest.Properties) != 1 || manifest.Genie == nil || !manifest.Genie.Hidden {
		t.Fatalf("unrelated manifest fields lost: %s %v", data, err)
	}
	for _, args := range [][]string{{"set", "view", "--group", "support"}, {"rm", "unknown"}, {"ls", "unknown"}, {"set", "shape"}, {"set", "shape", "--group", " "}, {"ls", "--remote", "https://example.invalid"}, {"ls", "--user", "member@example.com"}} {
		if _, err := run(args...); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, principal := range []string{"member@example.com", "account@example.com", "anonymous"} {
		t.Setenv("BASHY_PRINCIPAL", principal)
		for _, args := range [][]string{{"ls"}, {"set", "shape", "--group", "support"}, {"rm", "edit"}, {"ls", "--user", "owner@example.com"}} {
			if _, err := run(args...); err == nil {
				t.Fatalf("%s accepted %v", principal, args)
			}
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("refused commands mutated manifest")
	}
	t.Setenv("BASHY_PRINCIPAL", "owner@example.com")
	invalid := []byte(`{"scopes":{"unknown":{"groups":["support"]}}}`)
	if err := os.WriteFile(path, invalid, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := run("set", "shape", "--group", "support"); err == nil {
		t.Fatal("accepted an invalid manifest")
	}
	after, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(invalid, after) {
		t.Fatal("invalid manifest was overwritten")
	}
}

func TestShapeScopeUnknownOwnerRefusesRepresentedCaller(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, principal := range []string{"member@example.com", "account@example.com", "anonymous"} {
		t.Setenv("BASHY_PRINCIPAL", principal)
		if err := shapeScopeOwner(shape.App{}, &shapeCLIOptions{}); err == nil {
			t.Fatalf("accepted %s without a known owner", principal)
		}
	}
}
