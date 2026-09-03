package cli

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func TestBashPPLiveStatementReselection(t *testing.T) {
	for _, src := range []string{
		"set -o bashpp; type T int; echo enabled",
		"set -o bashpp\ntype T int\necho enabled\n",
	} {
		t.Run(src, func(t *testing.T) {
			var out bytes.Buffer
			r, err := interp.New(interp.Lang(syntax.LangBash), interp.StdIO(nil, &out, io.Discard), interp.Env(expand.ListEnviron()))
			if err != nil {
				t.Fatal(err)
			}
			if err := runStatementStream(context.Background(), r, []byte(src), syntax.LangBash, "bash"); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != "enabled\n" {
				t.Fatalf("output = %q", got)
			}
		})
	}
}

func TestBashPPLiveDisableFallsBackToClassic(t *testing.T) {
	var out, stderr bytes.Buffer
	r, err := interp.New(interp.Lang(syntax.LangBashPP), interp.StdIO(nil, &out, &stderr), interp.Env(expand.ListEnviron()))
	if err != nil {
		t.Fatal(err)
	}
	if err := runStatementStream(context.Background(), r, []byte("set +o bashpp; type T int; echo done"), syntax.LangBashPP, "bash"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "type: T: not found") || out.String() != "done\n" {
		t.Fatalf("stdout=%q stderr=%q", out.String(), stderr.String())
	}
}

func TestRunSessionCommandBashPPLive(t *testing.T) {
	var out, stderr bytes.Buffer
	status := RunSessionCommand(context.Background(), SessionIO{
		Command: "set +o bashpp; set -o bashpp; type T int; echo session",
		Env:     []string{"PATH="}, Stdout: &out, Stderr: &stderr,
	})
	if status != 0 || out.String() != "session\n" {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, out.String(), stderr.String())
	}
}

func TestBashPPInvocationSelectorIsTopLevelOnly(t *testing.T) {
	env := []string{"PATH=/bin", "BASHY_BASHPP=1", "HOME=/tmp"}
	got := consumeInvocationSelectors(slices.Clone(env))
	want := []string{"PATH=/bin", "HOME=/tmp"}
	if !slices.Equal(got, want) {
		t.Fatalf("consumed env = %q, want %q", got, want)
	}
	resolution, err := ResolveBashPP(BashPPSelector{
		Binary: BashPPBinaryBash,
		LookupEnv: func(name string) (string, bool) {
			if name == "BASHY_BASHPP" {
				return "1", true
			}
			return "", false
		},
	})
	if err != nil || !resolution.Enabled {
		t.Fatalf("top-level resolution = %+v, %v; want enabled", resolution, err)
	}
}

func TestLiveDialectStreamSelectionUsesParsedCommands(t *testing.T) {
	parse := func(t *testing.T, src string) *syntax.File {
		t.Helper()
		file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	if needsLiveDialectStream(parse(t, "echo 'set -o bashpp'\n"), syntax.LangBash) {
		t.Fatal("quoted data selected live streaming")
	}
	if !needsLiveDialectStream(parse(t, "set -o bashpp; echo live\n"), syntax.LangBash) {
		t.Fatal("parsed live toggle did not select streaming")
	}
	if needsLiveDialectStream(parse(t, "echo compatible\n"), syntax.LangBashPP) {
		t.Fatal("an active dialect without a transition selected streaming")
	}
}

func TestNeedsStdinExecRedirectIsASTBased(t *testing.T) {
	parse := func(t *testing.T, src string) *syntax.File {
		t.Helper()
		file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"exec fd0 input redirect", "echo start\nexec 0< file\n", true},
		{"nested exec fd0 redirect", "if true; then exec 0< file; fi\n", true},
		{"exec fd0 dup input", "exec 0<&3\n", true},
		{"comment mentioning exec 0<", "# exec 0< file\n", false},
		{"quoted string mentioning exec 0<", "echo 'exec 0< file'\n", false},
		{"here-document data mentioning exec 0<", "cat <<'EOF'\nexec 0< file\nEOF\n", false},
		{"non-exec command with fd0 redirect", "cat 0< file\n", false},
		{"exec redirecting another descriptor", "exec 1> file\n", false},
		{"exec with implicit stdin redirect", "exec < file\n", false},
	}
	for _, tc := range cases {
		if got := needsStdinExecRedirect(parse(t, tc.src)); got != tc.want {
			t.Errorf("%s (%q): needsStdinExecRedirect = %v, want %v",
				tc.name, tc.src, got, tc.want)
		}
	}
}
