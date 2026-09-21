package agentos

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/polyglot"
)

func TestTextRowsRegistered(t *testing.T) {
	for name, want := range map[string]struct {
		text, interpretedOnly bool
		verbs                 []string
	}{
		"k8s":     {true, false, []string{"apply", "delete", "get", "diff", "play", "down"}},
		"kube":    {true, false, []string{"apply"}},
		"helm":    {true, false, []string{"template", "install", "upgrade", "uninstall"}},
		"skill":   {true, true, []string{"run", "verify", "probe"}},
		"dag":     {true, true, nil},
		"compose": {true, false, nil},
		"tf":      {true, false, []string{"validate", "init", "plan", "apply", "destroy", "output"}},
		"python":  {false, false, nil},
	} {
		row, ok := polyglot.LookupLanguage(name)
		if !ok {
			t.Fatalf("%s: no row", name)
		}
		if row.Text != want.text || row.InterpretedOnly != want.interpretedOnly {
			t.Errorf("%s: Text=%v InterpretedOnly=%v", name, row.Text, row.InterpretedOnly)
		}
		if len(want.verbs) == 0 {
			continue
		}
		text, ok := row.NewRuntime(polyglot.RuntimeConfig{}).(polyglot.Text)
		if !ok {
			t.Fatalf("%s: runtime is %T", name, row.NewRuntime(polyglot.RuntimeConfig{}))
		}
		for _, verb := range want.verbs {
			found := false
			for _, v := range text.Verbs {
				found = found || v.Name == verb
			}
			if !found {
				t.Errorf("%s: verb %s missing", name, verb)
			}
		}
	}
}

func TestDagMethods(t *testing.T) {
	out, err := dagMethods(`{"status":"ok","result":{"file":"dag.md","tasks":[{"name":"build","lang":"bash"},{"name":"deploy","effects":["net","write"]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	exports, err := polyglot.ParseMethods("dag", out)
	if err != nil || len(exports) != 2 || exports[0].Name != "build" || len(exports[0].Effects) != 0 || strings.Join(exports[1].Effects, ",") != "net,write" {
		t.Fatalf("methods = %+v, %v (from %q)", exports, err, out)
	}
	if _, err := dagMethods("not json"); err == nil {
		t.Error("malformed list accepted")
	}
}

func TestManifestRowsRegistered(t *testing.T) {
	for name, verbs := range map[string][]string{
		"cargo": {"build", "test", "check", "run"}, "pyproject": {"sync", "run", "test", "build"}, "uv": {"sync"},
		"gomod": {"tidy", "build", "vet", "test", "run"}, "cmake": {"configure", "build", "test", "install"},
		"makefile": {"build", "test", "clean", "target"}, "make": {"build"}, "package": {"install", "run", "test", "build"}, "npm": {"run"},
	} {
		row, ok := polyglot.LookupLanguage(name)
		if !ok || !row.Text {
			t.Fatalf("%s: no text row", name)
		}
		text := row.NewRuntime(polyglot.RuntimeConfig{}).(polyglot.Text)
		for _, verb := range verbs {
			found := false
			for _, v := range text.Verbs {
				found = found || v.Name == verb
			}
			if !found {
				t.Errorf("%s: verb %s missing", name, verb)
			}
		}
		if text.WorkDir != "{cwd}" {
			t.Errorf("%s: manifest rows run in the caller's directory, got %q", name, text.WorkDir)
		}
	}
	if row, _ := polyglot.LookupLanguage("gomod"); row.ModuleFor != "go" {
		t.Errorf("gomod must provide the go module, ModuleFor=%q", row.ModuleFor)
	}
	if row, _ := polyglot.LookupLanguage("cargo"); len(row.NewRuntime(polyglot.RuntimeConfig{}).(polyglot.Text).Shadow) == 0 {
		t.Error("cargo must shadow its sources")
	}
}
