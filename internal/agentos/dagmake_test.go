// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/qiangli/bash/interp"
	"github.com/qiangli/bash/syntax"
)

const dagMakeFixture = "## Tasks\n\n### build\nCompile it.\n\n```bash\necho build\n```\n\n### test\nTest it.\nRequires: build\n\n```bash\necho test\n```\n"

const dagMakeFixtureHelp = "## Tasks\n\n### build\nCompile it.\n\n```bash\necho build\n```\n\n### help\nShow usage.\n\n```bash\necho usage\n```\n"

func writeDagMakeFixture(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseMakeArgs(t *testing.T) {
	base := string(filepath.Separator) + "base"
	sub := filepath.Join(base, "sub")
	for _, tc := range []struct {
		name    string
		args    []string
		wantOK  bool
		want    dagMakePlan
		wantDir string
	}{
		{"plain target", []string{"test"}, true, dagMakePlan{targets: []string{"test"}}, base},
		{"no args", nil, true, dagMakePlan{}, base},
		{"dry run", []string{"-n", "test"}, true, dagMakePlan{dry: true, targets: []string{"test"}}, base},
		{"long dry", []string{"--just-print", "test"}, true, dagMakePlan{dry: true, targets: []string{"test"}}, base},
		{"keep force silent", []string{"-k", "-B", "-s", "build"}, true, dagMakePlan{keep: true, force: true, quiet: true, targets: []string{"build"}}, base},
		{"bundled shorts", []string{"-skB", "build"}, true, dagMakePlan{keep: true, force: true, quiet: true, targets: []string{"build"}}, base},
		{"jobs attached", []string{"-j4", "test"}, true, dagMakePlan{jobs: 4, targets: []string{"test"}}, base},
		{"jobs separate", []string{"-j", "8", "test"}, true, dagMakePlan{jobs: 8, targets: []string{"test"}}, base},
		{"jobs bare", []string{"-j", "test"}, true, dagMakePlan{targets: []string{"test"}}, base},
		{"jobs long", []string{"--jobs=2", "test"}, true, dagMakePlan{jobs: 2, targets: []string{"test"}}, base},
		{"no print dir", []string{"--no-print-directory", "test"}, true, dagMakePlan{targets: []string{"test"}}, base},
		{"var override", []string{"VAR=1", "build"}, true, dagMakePlan{targets: []string{"build"}, overrides: []string{"VAR=1"}}, base},
		{"dashdash target", []string{"--", "-n"}, true, dagMakePlan{targets: []string{"-n"}}, base},
		{"-C", []string{"-C", "sub", "test"}, true, dagMakePlan{targets: []string{"test"}}, sub},
		{"-C attached", []string{"-Csub", "test"}, true, dagMakePlan{targets: []string{"test"}}, sub},
		{"--directory=", []string{"--directory=sub", "test"}, true, dagMakePlan{targets: []string{"test"}}, sub},
		{"-C in bundle", []string{"-sC", "sub", "test"}, true, dagMakePlan{quiet: true, targets: []string{"test"}}, sub},
		{"explicit -f", []string{"-f", "x.mk", "test"}, false, dagMakePlan{}, ""},
		{"--file=", []string{"--file=x.mk"}, false, dagMakePlan{}, ""},
		{"--help", []string{"--help"}, false, dagMakePlan{}, ""},
		{"--version", []string{"--version"}, false, dagMakePlan{}, ""},
		{"unknown long", []string{"--debug", "test"}, false, dagMakePlan{}, ""},
		{"unknown short", []string{"-r", "test"}, false, dagMakePlan{}, ""},
		{"question short", []string{"-q", "test"}, false, dagMakePlan{}, ""},
		{"-C missing arg", []string{"-C"}, false, dagMakePlan{}, ""},
	} {
		got, ok := parseMakeArgs(base, tc.args)
		if ok != tc.wantOK {
			t.Errorf("%s: parseMakeArgs(%v) ok=%v, want %v (plan %+v)", tc.name, tc.args, ok, tc.wantOK, got)
			continue
		}
		if !ok {
			continue
		}
		want := tc.want
		want.dir = tc.wantDir
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: plan = %+v, want %+v", tc.name, got, want)
		}
	}
}

func TestWantsDagMake(t *testing.T) {
	withDag := t.TempDir()
	writeDagMakeFixture(t, withDag, "dag.md", dagMakeFixture)
	withBoth := t.TempDir()
	writeDagMakeFixture(t, withBoth, "dag.md", dagMakeFixture)
	writeDagMakeFixture(t, withBoth, "Makefile", "all:\n")
	withLower := t.TempDir()
	writeDagMakeFixture(t, withLower, "dag.md", dagMakeFixture)
	writeDagMakeFixture(t, withLower, "makefile", "all:\n")
	withGNU := t.TempDir()
	writeDagMakeFixture(t, withGNU, "dag.md", dagMakeFixture)
	writeDagMakeFixture(t, withGNU, "GNUmakefile", "all:\n")
	noDag := t.TempDir()
	withHelp := t.TempDir()
	writeDagMakeFixture(t, withHelp, "dag.md", dagMakeFixtureHelp)

	argsOf := func(p dagMakePlan) string { return strings.Join(p.dagArgs, " ") }
	for _, tc := range []struct {
		name     string
		dir      string
		args     []string
		env      []string
		wantOK   bool
		wantList bool
		wantArgs string
	}{
		{"target", withDag, []string{"test"}, nil, true, false, "test"},
		{"flags and var", withDag, []string{"-n", "-k", "VAR=1", "build"}, nil, true, false, "--dry-run -k build VAR=1"},
		{"makefile present", withBoth, []string{"test"}, nil, false, false, ""},
		{"lower makefile present", withLower, []string{"test"}, nil, false, false, ""},
		{"GNUmakefile present", withGNU, []string{"test"}, nil, false, false, ""},
		{"no dag", noDag, []string{"test"}, nil, false, false, ""},
		{"-C into dag dir", filepath.Dir(withDag), []string{"-C", filepath.Base(withDag), "test"}, nil, true, false, "test"},
		{"-C into makefile dir", filepath.Dir(withBoth), []string{"-C", filepath.Base(withBoth), "test"}, nil, false, false, ""},
		{"MAKEFLAGS -f", withDag, []string{"test"}, []string{"MAKEFLAGS=-f other.mk"}, false, false, ""},
		{"MAKEFILES set", withDag, []string{"test"}, []string{"MAKEFILES=extra.mk"}, false, false, ""},
		{"no target lists", withDag, nil, nil, true, true, "--list"},
		{"help lists", withDag, []string{"help"}, nil, true, true, "--list"},
		{"dag help wins", withHelp, []string{"help"}, nil, true, false, "help"},
		{"-n alone is a dry default run", withDag, []string{"-n"}, nil, true, false, "--dry-run"},
		{"-n help is a dry run", withDag, []string{"-n", "help"}, nil, true, false, "--dry-run help"},
		{"missing dir", filepath.Join(withDag, "nope"), []string{"test"}, nil, false, false, ""},
	} {
		plan, ok := wantsDagMake(tc.dir, tc.args, tc.env)
		if ok != tc.wantOK {
			t.Errorf("%s: wantsDagMake ok=%v, want %v (plan %+v)", tc.name, ok, tc.wantOK, plan)
			continue
		}
		if !ok {
			continue
		}
		if plan.list != tc.wantList || argsOf(plan) != tc.wantArgs {
			t.Errorf("%s: list=%v args=%q, want list=%v args=%q", tc.name, plan.list, argsOf(plan), tc.wantList, tc.wantArgs)
		}
		if plan.dagPath != filepath.Join(plan.dir, "dag.md") {
			t.Errorf("%s: dagPath=%q, want under plan dir %q", tc.name, plan.dagPath, plan.dir)
		}
	}
}

type dagMakeCall struct {
	dir  string
	args []string
}

func stubDagMakeExec(t *testing.T, code int) *[]dagMakeCall {
	t.Helper()
	calls := &[]dagMakeCall{}
	old := dagMakeExec
	dagMakeExec = func(ctx context.Context, dir string, env []string, stdin io.Reader, stdout, stderr io.Writer, args []string) (int, error) {
		*calls = append(*calls, dagMakeCall{dir: dir, args: append([]string{}, args...)})
		fmt.Fprintf(stdout, "FAKE-DAG %s\n", strings.Join(args, " "))
		return code, nil
	}
	t.Cleanup(func() { dagMakeExec = old })
	return calls
}

// dagMakeHintEnv pins the two variables the hint reads from the process
// environment. A venue may export BASHY_HINTS=off (the gate lanes do) or run
// under an agent shell (BASHY_AGENTIC): the hint assertions must not depend
// on either.
func dagMakeHintEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BASHY_HINTS", "")
	t.Setenv("BASHY_AGENTIC", "")
}

func TestMakeFallthroughFrontDoor(t *testing.T) {
	dagMakeHintEnv(t)
	calls := stubDagMakeExec(t, 3)
	dir := t.TempDir()
	writeDagMakeFixture(t, dir, "dag.md", dagMakeFixture)

	var out, errOut bytes.Buffer
	code, handled := tryDagMakeFrontDoorAt(dir, []string{"-j2", "test"}, nil, nil, &out, &errOut)
	if !handled || code != 3 {
		t.Fatalf("handled=%v code=%d, want true/3", handled, code)
	}
	if len(*calls) != 1 {
		t.Fatalf("dag runs = %d, want 1", len(*calls))
	}
	call := (*calls)[0]
	if call.dir != dir || strings.Join(call.args, " ") != "-f "+filepath.Join(dir, "dag.md")+" -j 2 test" {
		t.Errorf("call = %+v", call)
	}
	if got := errOut.String(); got != "make test -> bashy dag test (dag.md)\n" {
		t.Errorf("hint = %q, want exactly one plain hint line", got)
	}

	// A Makefile in the directory: never handled, no dag run, no hint —
	// the applet path below stays exactly as it is today.
	writeDagMakeFixture(t, dir, "Makefile", "all:\n")
	out.Reset()
	errOut.Reset()
	if code, handled := tryDagMakeFrontDoorAt(dir, []string{"test"}, nil, nil, &out, &errOut); handled {
		t.Errorf("Makefile dir handled with code=%d", code)
	}
	if len(*calls) != 1 || errOut.Len() != 0 || out.Len() != 0 {
		t.Errorf("Makefile dir: calls=%d out=%q err=%q, want untouched", len(*calls), out.String(), errOut.String())
	}
}

func TestMakeFallthroughListAndHints(t *testing.T) {
	dagMakeHintEnv(t)
	calls := stubDagMakeExec(t, 0)
	dir := t.TempDir()
	writeDagMakeFixture(t, dir, "dag.md", dagMakeFixture)

	var out, errOut bytes.Buffer
	code, handled := tryDagMakeFrontDoorAt(dir, []string{"help"}, nil, nil, &out, &errOut)
	if !handled || code != 0 {
		t.Fatalf("help: handled=%v code=%d", handled, code)
	}
	if len(*calls) != 1 || !strings.HasSuffix(strings.Join((*calls)[0].args, " "), "--list") {
		t.Fatalf("help call = %+v, want a --list run", *calls)
	}
	if !strings.Contains(out.String(), "Run: bashy dag <target> | skill: bashy dag --skill DIR | more: bashy help\n") {
		t.Errorf("list pointer missing from stdout:\n%s", out.String())
	}
	if got := errOut.String(); got != "make help -> bashy dag --list (dag.md)\n" {
		t.Errorf("help hint = %q", got)
	}

	// Agent mode: the hint is one JSON record on stderr and the pointer
	// stays out of stdout (dag's own JSON envelope rides there).
	out.Reset()
	errOut.Reset()
	code, handled = tryDagMakeFrontDoorAt(dir, nil, []string{"BASHY_AGENTIC=1"}, nil, &out, &errOut)
	if !handled || code != 0 {
		t.Fatalf("agent list: handled=%v code=%d", handled, code)
	}
	if strings.Contains(out.String(), "Run: bashy dag") {
		t.Errorf("agent mode pointer leaked to stdout:\n%s", out.String())
	}
	hint := errOut.String()
	if strings.Count(hint, "\n") != 1 || !strings.Contains(hint, `"kind":"make-dag-fallthrough"`) || !strings.Contains(hint, "bashy dag --list") {
		t.Errorf("agent hint = %q, want one bashy-hint-v1 JSON line", hint)
	}

	// BASHY_HINTS=off silences the hint but not the run.
	out.Reset()
	errOut.Reset()
	if _, handled := tryDagMakeFrontDoorAt(dir, []string{"test"}, []string{"BASHY_HINTS=off"}, nil, &out, &errOut); !handled {
		t.Fatal("hints-off run not handled")
	}
	if errOut.Len() != 0 {
		t.Errorf("hints-off stderr = %q", errOut.String())
	}
}

// The shell path: `make` inside a bashy shell reaches the same fallthrough
// through the ExecHandler ring, and a Makefile directory passes through to
// the next handler with argv untouched.
func TestDagMakeHandlerShell(t *testing.T) {
	dagMakeHintEnv(t)
	calls := stubDagMakeExec(t, 0)
	dir := t.TempDir()
	writeDagMakeFixture(t, dir, "dag.md", dagMakeFixture)

	run := func(script string) (string, string) {
		var out, errOut bytes.Buffer
		// ServedInProcess mirrors wireExec: the applet name stays bare
		// (never hashed to a PATH file), which is what the handler keys on.
		r, err := interp.New(interp.StdIO(nil, &out, &errOut), interp.Dir(dir),
			interp.ServedInProcess(func(name string) bool { return name == "make" }),
			interp.ExecHandlers(dagMakeHandler()))
		if err != nil {
			t.Fatal(err)
		}
		prog, perr := syntax.NewParser().Parse(strings.NewReader(script), "")
		if perr != nil {
			t.Fatal(perr)
		}
		_ = r.Run(context.Background(), prog)
		return out.String(), errOut.String()
	}

	out, errOut := run("make test; echo status=$?\n")
	if !strings.Contains(out, "FAKE-DAG") || !strings.Contains(out, "status=0") {
		t.Errorf("intercepted run: out=%q err=%q", out, errOut)
	}
	if strings.Count(errOut, "-> bashy dag") != 1 {
		t.Errorf("hint count: err=%q", errOut)
	}
	if len(*calls) != 1 {
		t.Fatalf("dag runs = %d, want 1", len(*calls))
	}

	writeDagMakeFixture(t, dir, "Makefile", "test:\n\t@echo HOST-MAKE\n")
	out, errOut = run("make test; echo status=$?\n")
	if len(*calls) != 1 {
		t.Errorf("Makefile dir intercepted: calls=%d out=%q", len(*calls), out)
	}
	if strings.Contains(errOut, "-> bashy dag") {
		t.Errorf("Makefile dir hinted: err=%q", errOut)
	}
}

func TestMakeResolver(t *testing.T) {
	rc, ok := commandResolverFor(false)("make")
	if !ok {
		t.Fatal("make not resolved in agentic mode")
	}
	if !strings.Contains(rc.Desc, "in-process applet") || !strings.Contains(rc.Desc, "bashy dag") {
		t.Errorf("desc = %q", rc.Desc)
	}
	if rc.Path != "" || rc.Schema != nil {
		t.Errorf("path/schema = %q/%v, want bare-name resolution with no schema", rc.Path, rc.Schema)
	}
	// POSIX mode still names the applet, without promising the fallthrough
	// (the handler is not wired on that branch).
	rc, ok = commandResolverFor(true)("make")
	if !ok || !strings.Contains(rc.Desc, "in-process applet") || strings.Contains(rc.Desc, "forwards") {
		t.Errorf("posix desc = %q ok=%v", rc.Desc, ok)
	}
	// Certification runs measure the host: no resolver match at all.
	t.Setenv("VSC_PROFILE", "cert")
	if _, ok := commandResolverFor(false)("make"); ok {
		t.Error("make resolved under cert profile")
	}
}

// The Bash 5.3 gate binary (github.com/qiangli/bash/cmd/bash) must never
// reach any of this: it lives in the bash module, whose import graph is
// disjoint from this module's internal/agentos (and Go's internal rule
// forbids the import outright). This makes the disjointness executable
// beside the feature, complementing TestClassicBashImportGraphRatchet.
func TestDagMakeUnreachableFromClassicBash(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/qiangli/bash/cmd/bash").CombinedOutput()
	if err != nil {
		t.Skipf("go list unavailable (%v): %s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.HasPrefix(pkg, "github.com/qiangli/bashy/") {
			t.Errorf("cmd/bash transitively imports %s — dagMakeHandler and the make resolver live in bashy/internal/agentos and must stay unreachable from the drop-in", pkg)
		}
	}
}
