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
