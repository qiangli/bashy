package tmpl

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// stubEngine is a compliant engine: it lists render (declaring exec) and
// renders by wrapping the data.
func stubEngine(_ context.Context, argv []string, _ io.Writer) (string, error) {
	switch argv[0] {
	case "methods":
		return `{"name":"render","effects":["exec"]}` + "\n", nil
	case "render":
		return "stub[" + argv[2] + "]", nil
	}
	return "", errors.New("unknown verb")
}

func withEngines(t *testing.T, engines map[string]Engine) {
	t.Helper()
	saved := ResolveEngine
	ResolveEngine = func(name string) (Engine, bool) { e, ok := engines[name]; return e, ok }
	t.Cleanup(func() { ResolveEngine = saved })
}

func TestEngineRendersThroughCompliantStub(t *testing.T) {
	withEngines(t, map[string]Engine{"stub": stubEngine})
	f := writeTmpl(t, "ignored by the stub")
	code, out, errs := run("", "render", "--engine", "stub", f, "--data", `{"a":1}`)
	if code != 0 || out != `stub[{"a":1}]` {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	code, out, _ = run("", "render", "--engine=stub", f)
	if code != 0 || out != "stub[{}]" {
		t.Fatalf("no data: code=%d out=%q", code, out)
	}
}

func TestEngineGoMatchesDefault(t *testing.T) {
	withEngines(t, map[string]Engine{"stub": stubEngine})
	f := writeTmpl(t, "hello {{ upper .name }}!")
	_, want, _ := run("", "render", f, "--data", `{"name":"x"}`)
	for _, args := range [][]string{
		{"render", "--engine", "go", f, "--data", `{"name":"x"}`},
		{"render", f, "--engine=go", "--data", `{"name":"x"}`},
	} {
		code, got, errs := run("", args...)
		if code != 0 || got != want || want != "hello X!" {
			t.Fatalf("%v: code=%d got=%q want=%q err=%q", args, code, got, want, errs)
		}
	}
}

func TestEngineNonCompliantIsRefusedNeverFallsBack(t *testing.T) {
	f := writeTmpl(t, "hello {{ upper .name }}!")
	cases := map[string]Engine{
		"no-methods": func(_ context.Context, argv []string, _ io.Writer) (string, error) {
			if argv[0] == "methods" {
				return "", nil
			}
			return "rendered", nil
		},
		"no-render": func(_ context.Context, argv []string, _ io.Writer) (string, error) {
			if argv[0] == "methods" {
				return `{"name":"lint"}` + "\n", nil
			}
			return "rendered", nil
		},
		"methods-fails": func(_ context.Context, argv []string, _ io.Writer) (string, error) {
			return "", errors.New("exited 2")
		},
		"methods-garbage": func(_ context.Context, argv []string, _ io.Writer) (string, error) {
			return "not json\n", nil
		},
	}
	withEngines(t, cases)
	for name := range cases {
		code, out, errs := run("", "render", "--engine", name, f, "--data", `{"name":"x"}`)
		if code == 0 || out != "" || !strings.Contains(errs, "engine is not template-compliant") {
			t.Errorf("%s: code=%d out=%q err=%q", name, code, out, errs)
		}
		if strings.Contains(out, "HELLO") || strings.Contains(out, "hello X") {
			t.Errorf("%s fell back to the Go engine: %q", name, out)
		}
	}
}

func TestEngineRenderFailureIsAnError(t *testing.T) {
	withEngines(t, map[string]Engine{"boom": func(_ context.Context, argv []string, _ io.Writer) (string, error) {
		if argv[0] == "methods" {
			return `{"name":"render"}`, nil
		}
		return "partial", errors.New("exited 3")
	}})
	f := writeTmpl(t, "x")
	code, out, errs := run("", "render", "--engine", "boom", f)
	if code != 1 || out != "" || !strings.Contains(errs, "exited 3") || strings.Contains(errs, "not template-compliant") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
}

func TestEngineUnknownIsRefusedAndDataIsChecked(t *testing.T) {
	withEngines(t, map[string]Engine{"stub": stubEngine})
	f := writeTmpl(t, "x")
	code, _, errs := run("", "render", "--engine", "nope", f)
	if code != 1 || !strings.Contains(errs, "PATH is never consulted") {
		t.Fatalf("unknown: code=%d err=%q", code, errs)
	}
	code, _, errs = run("", "render", "--engine", "stub", f, "--data", `[1]`)
	if code != 1 || !strings.Contains(errs, "JSON object") {
		t.Fatalf("data: code=%d err=%q", code, errs)
	}
	code, _, errs = run("", "render", "--engine")
	if code != 2 || !strings.Contains(errs, "--engine needs a value") {
		t.Fatalf("flag: code=%d err=%q", code, errs)
	}
}

func TestHelpDocumentsTheEngineContract(t *testing.T) {
	_, out, _ := run("", "--help")
	for _, want := range []string{"--engine", "ENGINE methods FILE", "ENGINE render FILE DATA_JSON", "engine is not template-compliant", "never a program found on PATH"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q", want)
		}
	}
}
