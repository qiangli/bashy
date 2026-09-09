package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestGoSourceVersionSelection(t *testing.T) {
	for _, flags := range [][]string{{"--go-version=go1.12"}, {"--go-version", "go1.12"}} {
		args := append([]string{"bashy"}, flags...)
		args = append(args, "--source=go", "--bashpp", "--check", "original.go", "--go-version=argument")
		remaining, sel, err := stripGoSourceInvocationFlags(args)
		if err != nil || sel.GoVersion != "go1.12" || !sel.GoVersionSeen {
			t.Fatalf("selection: %+v %v", sel, err)
		}
		if !reflect.DeepEqual(remaining, []string{"bashy", "--bashpp", "original.go", "--go-version=argument"}) {
			t.Fatalf("operand/program argument boundary changed: %v", remaining)
		}
		res, err := ResolveGoSource(sel, GoSourceContext{Binary: BashPPBinaryBashy, BashPP: true})
		if err != nil || res.GoVersion != "go1.12" || !res.Check {
			t.Fatalf("resolution: %+v %v", res, err)
		}
	}
	for _, args := range [][]string{{"bashy", "--go-version"}, {"bashy", "--go-version="}} {
		if _, _, err := stripGoSourceInvocationFlags(args); err == nil {
			t.Fatal("missing version accepted")
		}
	}
	if _, err := ResolveGoSource(GoSourceSelection{GoVersion: "go1.12", GoVersionSeen: true}, GoSourceContext{Binary: BashPPBinaryBashy, BashPP: true}); err == nil || !strings.Contains(err.Error(), "requires --source=go") {
		t.Fatalf("language version leaked to shell source: %v", err)
	}
}

func TestRunGoSourceVersionReachesLoader(t *testing.T) {
	path := writeGoFixture(t, "package p\nvar _ = 0b101\n")
	withGoSourceSelection(t, GoSourceResolution{Enabled: true, Check: true, Files: []string{path}, GoVersion: "go1.12"})
	called := false
	withGoSourceHook(t, func(_ []GoSourceFile, opts GoSourceOptions) (*GoSourceProgram, error) {
		called = true
		if opts.RunMain || opts.GoVersion != "go1.12" {
			t.Fatalf("wrong checker options: %+v", opts)
		}
		return &GoSourceProgram{Package: "p"}, nil
	})
	if err := runGoSourceInvocation(); err != nil || !called {
		t.Fatalf("load: called=%v err=%v", called, err)
	}
}
