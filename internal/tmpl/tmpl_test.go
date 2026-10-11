package tmpl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTmpl(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.tmpl")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func run(stdin string, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Main(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestRenderDataFromArgStdinAndFile(t *testing.T) {
	f := writeTmpl(t, "hello {{ upper .name }}!")
	dataFile := filepath.Join(t.TempDir(), "d.json")
	if err := os.WriteFile(dataFile, []byte(`{"name":"file"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		stdin string
		args  []string
		want  string
	}{
		"arg":        {"", []string{"render", f, "--data", `{"name":"arg"}`}, "hello ARG!"},
		"arg-equals": {"", []string{"render", f, `--data={"name":"eq"}`}, "hello EQ!"},
		"stdin":      {`{"name":"stdin"}`, []string{"render", f, "--data", "-"}, "hello STDIN!"},
		"file":       {"", []string{"render", f, "--data-file", dataFile}, "hello FILE!"},
	} {
		code, out, errs := run(tc.stdin, tc.args...)
		if code != 0 || out != tc.want {
			t.Errorf("%s: code=%d out=%q err=%q, want %q", name, code, out, errs, tc.want)
		}
	}
}

func TestRenderMissingKeyIsAnError(t *testing.T) {
	f := writeTmpl(t, "{{ .absent }}")
	code, out, errs := run("", "render", f, "--data", `{"name":"x"}`)
	if code != 1 || out != "" || !strings.Contains(errs, "absent") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	if code, _, errs := run("", "render", f); code != 1 || !strings.Contains(errs, "absent") {
		t.Fatalf("no data: code=%d err=%q", code, errs)
	}
}

func TestRenderRefusesImpureFunctionsAtParseTime(t *testing.T) {
	for _, fn := range []string{"exec", "env", "readFile", "getenv", "httpGet"} {
		f := writeTmpl(t, "{{ "+fn+` "x" }}`)
		code, out, errs := run("", "render", f, "--data", "{}")
		if code != 1 || out != "" || !strings.Contains(errs, "function \""+fn+"\" not defined") {
			t.Errorf("%s: code=%d out=%q err=%q", fn, code, out, errs)
		}
	}
}

func TestRenderHelpers(t *testing.T) {
	src := `{{ lower "AbC" }}|{{ trim "  a " }}|{{ replace "a" "b" "banana" }}|{{ join "," (split " " "x y z") }}|` +
		`{{ join "-" .list }}|{{ toJson .obj }}|{{ default "dflt" .empty }}|{{ default "dflt" .name }}|` +
		"{{ indent 2 \"a\\nb\" }}|{{ contains \"an\" \"banana\" }}"
	got, err := Render("t", src, []byte(`{"list":["a",1],"obj":{"k":"<v>"},"empty":"","name":"n"}`))
	want := `abc|a|bbnbnb|x,y,z|a-1|{"k":"<v>"}|dflt|n|  a` + "\n  b|true"
	if err != nil || got != want {
		t.Fatalf("got %q err %v\nwant %q", got, err, want)
	}
}

func TestRenderRejectsBadData(t *testing.T) {
	f := writeTmpl(t, "x")
	for _, d := range []string{`not json`, `[1,2]`} {
		if code, _, errs := run("", "render", f, "--data", d); code != 1 || errs == "" {
			t.Errorf("data %q: code=%d err=%q", d, code, errs)
		}
	}
	if code, _, _ := run("", "render", f, "--data", "{}", "--data-file", f); code != 2 {
		t.Error("--data with --data-file accepted")
	}
	if code, _, _ := run("", "render"); code != 2 {
		t.Error("missing FILE accepted")
	}
}

func TestHelpStatesThePurityRule(t *testing.T) {
	code, out, _ := run("", "--help")
	if code != 0 || !strings.Contains(out, "Purity rule") || !strings.Contains(out, "no exec, env, file or network") {
		t.Fatalf("help: code=%d %q", code, out)
	}
}
