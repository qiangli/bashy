package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/polyglot"
	"mvdan.cc/sh/v3/syntax"
)

const tmplFixtureBody = "hello {{ upper .name }}"

func tmplFenceRun(t *testing.T, script string) (string, string) {
	t.Helper()
	saved := polyglot.ToolResolver
	polyglot.ToolResolver = islandToolResolver
	defer func() { polyglot.ToolResolver = saved }()
	err, out, diag := runDecorated(t, t.Context(), syntax.LangBashPP, script, map[string]string{"BASHY_AUDIT": "0", tmplTestChildEnv: "1"})
	if err != nil {
		t.Fatalf("run: %v\nout=%q diag=%q", err, out, diag)
	}
	return out.String(), diag.String()
}

func TestTmplFenceRenders(t *testing.T) {
	out, _ := tmplFenceRun(t, "~~~tmpl as T\nhello {{ upper .name }}\n~~~\nr := T.render('{\"name\":\"x\"}')\necho \"$r\"\n")
	if strings.TrimSpace(out) != "hello X" {
		t.Fatalf("out=%q", out)
	}
}

func TestTmplFenceMethodEffectsAreRead(t *testing.T) {
	row, ok := polyglot.LookupLanguage("tmpl")
	if !ok || !row.Text {
		t.Fatalf("tmpl row: %+v ok=%v", row, ok)
	}
	if alias, ok := polyglot.LookupLanguage("gotmpl"); !ok || alias.Canonical != "tmpl" {
		t.Fatalf("gotmpl alias: %+v ok=%v", alias, ok)
	}
	text, ok := row.NewRuntime(polyglot.RuntimeConfig{}).(polyglot.Text)
	if !ok {
		t.Fatalf("runtime is %T", row.NewRuntime(polyglot.RuntimeConfig{}))
	}
	if len(text.Verbs) != 1 || text.Verbs[0].Name != "render" || strings.Join(text.Verbs[0].Effects, ",") != "read" ||
		strings.Join(text.Verbs[0].Args, " ") != "render {file} --data" || text.Tool != "tmpl" || text.FileName != "fence.tmpl" {
		t.Fatalf("row = %+v", text)
	}
}

func TestTmplFenceGuard(t *testing.T) {
	savedGate := interp.ForeignEffectGate
	interp.ForeignEffectGate = fenceEffectGate
	defer func() { interp.ForeignEffectGate = savedGate }()
	script := func(effects string) string {
		return "~~~tmpl as T\nhi\n~~~\n@guard(effects: \"" + effects + "\")\nfunction f() { r := T.render('{}'); echo \"$r\"; }\nf\n"
	}
	saved := polyglot.ToolResolver
	polyglot.ToolResolver = islandToolResolver
	defer func() { polyglot.ToolResolver = saved }()
	env := map[string]string{"BASHY_AUDIT": "0", tmplTestChildEnv: "1"}
	_, out, diag := runDecorated(t, t.Context(), syntax.LangBashPP, script("write"), env)
	if !strings.Contains(out.String()+diag.String(), "exceed the guard") || strings.Contains(out.String(), "hi") {
		t.Fatalf("a cap without read must deny: out=%q diag=%q", out, diag)
	}
	_, out, diag = runDecorated(t, t.Context(), syntax.LangBashPP, script("read"), env)
	if strings.TrimSpace(out.String()) != "hi" {
		t.Fatalf("a read cap must allow: out=%q diag=%q", out, diag)
	}
}

func TestTmplEmbedMatchesFence(t *testing.T) {
	dir := newFenceEmbedFixtureDir(t)
	if err := os.WriteFile(filepath.Join(dir, "fixture.tmpl"), []byte(tmplFixtureBody), 0o600); err != nil {
		t.Fatal(err)
	}
	rel := fenceEmbedRelPath(t, filepath.Join(dir, "fixture.tmpl"))
	out, _ := tmplFenceRun(t, "embed tmpl \"./"+rel+"\" as T\nr := T.render('{\"name\":\"x\"}')\necho \"$r\"\n")
	if strings.TrimSpace(out) != "hello X" {
		t.Fatalf("embed out=%q", out)
	}
}
