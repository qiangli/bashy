package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/polyglot"
	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/bashy/internal/tmpl"
	"github.com/qiangli/yoke/pkg/fleet"
)

const stubEngineScript = `case "$1" in
methods) printf '%s\n' '{"name":"render","effects":["exec"]}' ;;
render) printf 'reg[%s]' "$3" ;;
esac`

const stubShellFunctionEngine = `eng() {
  case "$1" in
    methods) printf '%s\n' '{"name":"render","effects":["exec"]}' ;;
    render) printf 'fn[%s]' "$3" ;;
  esac
}
`

const stubBashSharpFunctionEngine = `func eng(verb string, file string, args ...string) string {
  if verb == "methods" {
    return "{\"name\":\"render\"}"
  }
  return "bashsharp"
}
`

// tmplEngineFence runs a script with the engine seams installed and the
// whole run's outcome (error + diagnostics) folded into one string.
func tmplEngineFence(t *testing.T, script string) (out, failure string) {
	t.Helper()
	saved := polyglot.ToolResolver
	polyglot.ToolResolver = islandToolResolver
	defer func() { polyglot.ToolResolver = saved }()
	installFenceSeams()
	err, stdout, diag := runDecorated(t, t.Context(), syntax.LangBashPP, script, map[string]string{"BASHY_AUDIT": "0", tmplTestChildEnv: "1"})
	failure = diag.String()
	if err != nil {
		failure += err.Error()
	}
	return stdout.String(), failure
}

func registerStubEngine(t *testing.T, name, script string) {
	t.Helper()
	ringDir(t)
	writeRecord(t, fleet.Command{Name: name, Exec: []string{shPath(t), "-c", script, name, fleet.ArgsToken}})
}

func TestTmplFenceEngineFunction(t *testing.T) {
	out, failure := tmplEngineFence(t, stubShellFunctionEngine+"~~~tmpl as T !eng\nhello\n~~~\nr := T.render('{\"a\":1}')\necho \"$r\"\n")
	if strings.TrimSpace(out) != `fn[{"a":1}]` {
		t.Fatalf("shell function: out=%q failure=%q", out, failure)
	}
	out, failure = tmplEngineFence(t, stubBashSharpFunctionEngine+"~~~tmpl as T !eng\nhello\n~~~\nr := T.render('{}')\necho \"$r\"\n")
	if strings.TrimSpace(out) != "bashsharp" {
		t.Fatalf("Bash# function: out=%q failure=%q", out, failure)
	}
}

func TestTmplFenceEngineRegisteredCommand(t *testing.T) {
	registerStubEngine(t, "stubtmpl", stubEngineScript)
	out, failure := tmplEngineFence(t, "~~~tmpl as T !stubtmpl\nhello\n~~~\nr := T.render('{\"a\":1}')\necho \"$r\"\n")
	if strings.TrimSpace(out) != `reg[{"a":1}]` {
		t.Fatalf("out=%q failure=%q", out, failure)
	}
}

func TestTmplFenceEngineNonCompliantIsRefused(t *testing.T) {
	ringDir(t)
	writeRecord(t, fleet.Command{Name: "norender", Exec: []string{shPath(t), "-c", `[ "$1" = methods ] && printf '%s\n' '{"name":"lint"}'; exit 0`, "norender", fleet.ArgsToken}})
	writeRecord(t, fleet.Command{Name: "nomethods", Exec: []string{shPath(t), "-c", `exit 0`, "nomethods", fleet.ArgsToken}})
	for engine, want := range map[string]string{"norender": "package T is not imported", "nomethods": "declared no methods"} {
		out, failure := tmplEngineFence(t, "~~~tmpl as T !"+engine+"\nhello {{ upper .name }}\n~~~\nT.render('{\"name\":\"x\"}')\necho after\n")
		if !strings.Contains(failure, want) || out != "" {
			t.Errorf("%s: out=%q failure=%q (want %q, no Go fallback)", engine, out, failure, want)
		}
	}
	out, failure := tmplEngineFence(t, "~~~tmpl as T !PATHONLY\nhello\n~~~\nr := T.render('{}')\necho \"$r\"\n")
	if !strings.Contains(failure, "PATH is never consulted") || out != "" {
		t.Errorf("unregistered engine: out=%q failure=%q", out, failure)
	}
}

func TestTmplFenceEngineEffectsAreGuardable(t *testing.T) {
	script := func(cap string) string {
		return stubShellFunctionEngine + "~~~tmpl as T !eng\nhi\n~~~\n@guard(effects: \"" + cap + "\")\nfunction f() { r := T.render('{}'); echo \"$r\"; }\nf\n"
	}
	out, failure := tmplEngineFence(t, script("read"))
	if !strings.Contains(failure, "exceed the guard") || strings.Contains(out, "fn[") {
		t.Fatalf("a read-only cap must deny a declared exec engine: out=%q failure=%q", out, failure)
	}
	out, failure = tmplEngineFence(t, script("exec"))
	if strings.TrimSpace(out) != "fn[{}]" {
		t.Fatalf("an exec cap must allow: out=%q failure=%q", out, failure)
	}
}

func runTmplCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := tmpl.Main(args, strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

func TestTmplCLIEngineRegisteredCommand(t *testing.T) {
	registerStubEngine(t, "stubtmpl", stubEngineScript)
	file := writeFixture(t, "hello {{ upper .name }}!")
	code, out, errs := runTmplCLI("render", "--engine", "stubtmpl", file, "--data", `{"name":"x"}`)
	if code != 0 || out != `reg[{"name":"x"}]` {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	// --engine go is the built-in engine, byte-identical to the default.
	_, want, _ := runTmplCLI("render", file, "--data", `{"name":"x"}`)
	code, got, errs := runTmplCLI("render", "--engine", "go", file, "--data", `{"name":"x"}`)
	if code != 0 || got != want || want != "hello X!" {
		t.Fatalf("go: code=%d got=%q want=%q err=%q", code, got, want, errs)
	}
}

func TestTmplCLIEngineRefusals(t *testing.T) {
	ringDir(t)
	writeRecord(t, fleet.Command{Name: "norender", Exec: []string{shPath(t), "-c", `[ "$1" = methods ] && printf '%s\n' '{"name":"lint"}'; exit 0`, "norender", fleet.ArgsToken}})
	writeRecord(t, fleet.Command{Name: "nomethods", Exec: []string{shPath(t), "-c", `exit 0`, "nomethods", fleet.ArgsToken}})
	file := writeFixture(t, "hello {{ upper .name }}!")
	if !fenceRunnerRegistered("sprint") {
		t.Fatal("explain must be a bashy-dispatched verb")
	}
	for _, engine := range []string{"norender", "nomethods", "sprint"} {
		code, out, errs := runTmplCLI("render", "--engine", engine, file, "--data", `{"name":"x"}`)
		if code != 1 || out != "" || !strings.Contains(errs, "engine is not template-compliant") {
			t.Errorf("%s: code=%d out=%q err=%q", engine, code, out, errs)
		}
	}
	code, _, errs := runTmplCLI("render", "--engine", "PATHONLY", file)
	if code != 1 || !strings.Contains(errs, "PATH is never consulted") {
		t.Errorf("unregistered: code=%d err=%q", code, errs)
	}
}

func writeFixture(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.tmpl")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
