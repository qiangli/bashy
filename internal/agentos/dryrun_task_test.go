package agentos

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Exercise the product wiring, including the always-installed dry-run policy,
// rather than constructing an engine with native handlers alone.
func TestWireExecDryRunTaskRedirection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native cooperative task file opens are Unix-only")
	}
	t.Setenv("BASHY_HINTS", "off")
	t.Setenv("BASHY_ADVISOR", "off")
	t.Setenv("BASHY_AGENTIC", "0")
	for _, tc := range []struct {
		name          string
		initial       bool
		prefix        string
		task, refused bool
		want          string
	}{
		{name: "normal-task", task: true, want: "native"},
		{name: "initial-dryrun", initial: true, want: "original"},
		{name: "runtime-dryrun", prefix: "set -o dryrun;", want: "original"},
		{name: "runtime-return-to-normal", initial: true, prefix: "set +o dryrun;", task: true, want: "native"},
		{name: "initial-dryrun-task-refused", initial: true, task: true, refused: true, want: "original"},
		{name: "runtime-dryrun-task-refused", prefix: "set -o dryrun;", task: true, refused: true, want: "original"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target")
			if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			var out, diagnostic strings.Builder
			opts := []interp.RunnerOption{interp.Dir(dir), interp.Lang(syntax.LangBashPP), interp.Env(expand.ListEnviron(os.Environ()...))}
			opts = wireExec(opts, false, os.Environ(), nil, &out, &diagnostic, tc.initial)
			r, err := interp.New(opts...)
			if err != nil {
				t.Fatal(err)
			}
			body := "printf native > target"
			if tc.task {
				body = `func write(done) { printf native > target || return $?; done <- true; }
 done := make(chan bool)
 go write(done)
 completed := <-done
 `
			}
			f, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(tc.prefix+body), "")
			if err != nil {
				t.Fatal(err)
			}
			err = r.Run(context.Background(), f)
			if tc.refused {
				if err == nil || !strings.Contains(diagnostic.String(), "custom open handlers are unavailable inside a Bash++ task") {
					t.Fatalf("err=%v diagnostic=%q", err, diagnostic.String())
				}
			} else if err != nil {
				t.Fatalf("err=%v diagnostic=%q", err, diagnostic.String())
			}
			got, readErr := os.ReadFile(target)
			if readErr != nil || string(got) != tc.want {
				t.Fatalf("file=%q err=%v; want %q", got, readErr, tc.want)
			}
		})
	}
}
