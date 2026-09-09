// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package cli

import (
	"errors"
	"flag"
	"go/build"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Sprint 118, W1: tests for the Go-source selector. This package is the SHARED
// shell core, so the front-end hooks are nil here exactly as they are in the
// pure bash drop-in: these tests prove the selection, the refusals, the argv
// scanning and the input collection. Loading real Go source is proved against
// the wired front end in internal/agentos, which is where the import lives.

func TestStripGoSourceInvocationFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantArgs []string
		wantSel  GoSourceSelection
	}{
		{
			name:     "no selector is untouched",
			args:     []string{"bashy", "--bashpp", "script.bpp", "arg"},
			wantArgs: []string{"bashy", "--bashpp", "script.bpp", "arg"},
		},
		{
			name:     "--source=go is consumed",
			args:     []string{"bashy", "--source=go", "hello.go"},
			wantArgs: []string{"bashy", "hello.go"},
			wantSel:  GoSourceSelection{Language: "go", LanguageSeen: true},
		},
		{
			name:     "separated --source go is consumed with its value",
			args:     []string{"bashy", "--source", "go", "hello.go"},
			wantArgs: []string{"bashy", "hello.go"},
			wantSel:  GoSourceSelection{Language: "go", LanguageSeen: true},
		},
		{
			name:     "--check and repeated --go-file are consumed in order",
			args:     []string{"bashy", "--source=go", "--check", "--go-file", "b.go", "--go-file=a.go"},
			wantArgs: []string{"bashy"},
			wantSel: GoSourceSelection{
				Language: "go", LanguageSeen: true, Check: true,
				Files: []string{"b.go", "a.go"},
			},
		},
		{
			name:     "last --source wins",
			args:     []string{"bashy", "--source=go", "--source=sh"},
			wantArgs: []string{"bashy"},
			wantSel:  GoSourceSelection{Language: "sh", LanguageSeen: true},
		},
		{
			// The script's own arguments must survive verbatim: a Go program
			// is entitled to be passed the word --check.
			name:     "scanning stops at the operand",
			args:     []string{"bashy", "--source=go", "hello.go", "--check", "--source=sh"},
			wantArgs: []string{"bashy", "hello.go", "--check", "--source=sh"},
			wantSel:  GoSourceSelection{Language: "go", LanguageSeen: true},
		},
		{
			name:     "scanning stops at -c",
			args:     []string{"bashy", "--source=go", "-c", "--check"},
			wantArgs: []string{"bashy", "-c", "--check"},
			wantSel:  GoSourceSelection{Language: "go", LanguageSeen: true},
		},
		{
			name:     "scanning stops at --",
			args:     []string{"bashy", "--", "--source=go"},
			wantArgs: []string{"bashy", "--", "--source=go"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, sel, err := stripGoSourceInvocationFlags(tc.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(got, tc.wantArgs) {
				t.Errorf("args = %q, want %q", got, tc.wantArgs)
			}
			if sel.Language != tc.wantSel.Language || sel.LanguageSeen != tc.wantSel.LanguageSeen ||
				sel.Check != tc.wantSel.Check || !slices.Equal(sel.Files, tc.wantSel.Files) {
				t.Errorf("selection = %+v, want %+v", sel, tc.wantSel)
			}
		})
	}
}

func TestStripGoSourceInvocationFlagsMissingValue(t *testing.T) {
	for _, args := range [][]string{
		{"bashy", "--source"},
		{"bashy", "--go-file"},
	} {
		if _, _, err := stripGoSourceInvocationFlags(args); err == nil {
			t.Errorf("%q: want an error for a missing value", args)
		} else if !IsGoSourceError(err) {
			t.Errorf("%q: error %v is not a Go-source refusal", args, err)
		}
	}
}

func TestResolveGoSourceRefusals(t *testing.T) {
	bashy := GoSourceContext{Binary: BashPPBinaryBashy, BashPP: true}
	goSel := GoSourceSelection{Language: "go", LanguageSeen: true}
	tests := []struct {
		name string
		sel  GoSourceSelection
		ctx  GoSourceContext
		want string
	}{
		{
			name: "unknown language",
			sel:  GoSourceSelection{Language: "rust", LanguageSeen: true},
			ctx:  bashy,
			want: `bashy: --source: unknown input language "rust" (expected "sh" or "go")`,
		},
		{
			name: "go without Bash++",
			sel:  goSel,
			ctx:  GoSourceContext{Binary: BashPPBinaryBashy},
			want: "bashy: --source=go requires --bashpp",
		},
		{
			name: "go under POSIX",
			sel:  goSel,
			ctx:  GoSourceContext{Binary: BashPPBinaryBashy, BashPP: true, Posix: true},
			want: "bashy: --source=go is not available in POSIX mode",
		},
		{
			// The POSIX conformance refusal must win even when Bash++ was
			// forced inert by that same POSIX selection.
			name: "go under POSIX on the bash drop-in reports POSIX, not bashpp",
			sel:  goSel,
			ctx:  GoSourceContext{Binary: BashPPBinaryBash, Posix: true},
			want: "bashy: --source=go is not available in POSIX mode",
		},
		{
			name: "go on the bash drop-in",
			sel:  goSel,
			ctx:  GoSourceContext{Binary: BashPPBinaryBash, BashPP: true},
			want: "bashy: --source=go requires the bashy front door",
		},
		{
			name: "check without go",
			sel:  GoSourceSelection{Check: true},
			ctx:  bashy,
			want: "bashy: --check requires --source=go",
		},
		{
			name: "check with an explicit --source=sh",
			sel:  GoSourceSelection{Language: "sh", LanguageSeen: true, Check: true},
			ctx:  bashy,
			want: "bashy: --check requires --source=go",
		},
		{
			name: "go-file without go",
			sel:  GoSourceSelection{Files: []string{"a.go"}},
			ctx:  bashy,
			want: "bashy: --go-file requires --source=go",
		},
		{
			name: "go-file plus an operand is ambiguous",
			sel:  GoSourceSelection{Language: "go", LanguageSeen: true, Files: []string{"a.go"}},
			ctx:  GoSourceContext{Binary: BashPPBinaryBashy, BashPP: true, HasOperand: true},
			want: "bashy: --go-file cannot be combined with a file operand",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ResolveGoSource(tc.sel, tc.ctx)
			if err == nil {
				t.Fatalf("want refusal %q, got resolution %+v", tc.want, res)
			}
			if !IsGoSourceError(err) {
				t.Errorf("error %v is not a Go-source refusal", err)
			}
			if err.Error() != tc.want {
				t.Errorf("error = %q, want %q", err.Error(), tc.want)
			}
			if res.Enabled {
				t.Error("a refused selection must not report Enabled")
			}
		})
	}
}

func TestResolveGoSourceAccepts(t *testing.T) {
	bashy := GoSourceContext{Binary: BashPPBinaryBashy, BashPP: true}
	t.Run("go with bashpp", func(t *testing.T) {
		res, err := ResolveGoSource(GoSourceSelection{Language: "go", LanguageSeen: true}, bashy)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Enabled || res.Check {
			t.Fatalf("resolution = %+v", res)
		}
	})
	t.Run("go with check", func(t *testing.T) {
		res, err := ResolveGoSource(
			GoSourceSelection{Language: "go", LanguageSeen: true, Check: true}, bashy)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Enabled || !res.Check {
			t.Fatalf("resolution = %+v", res)
		}
	})
	t.Run("explicit sh is the unchanged shell path", func(t *testing.T) {
		res, err := ResolveGoSource(GoSourceSelection{Language: "sh", LanguageSeen: true}, bashy)
		if err != nil {
			t.Fatal(err)
		}
		if res.Enabled {
			t.Fatalf("--source=sh must not enable Go input: %+v", res)
		}
	})
	t.Run("no selector at all", func(t *testing.T) {
		res, err := ResolveGoSource(GoSourceSelection{}, bashy)
		if err != nil || res.Enabled {
			t.Fatalf("res = %+v, err = %v", res, err)
		}
	})
}

func TestCollectGoSourcesSingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.go")
	body := "package main\n\nfunc main() { println(\"hi\") }\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	in, err := CollectGoSources(GoSourceResolution{Enabled: true}, path, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Files) != 1 || in.Files[0].Name != path {
		t.Fatalf("files = %+v", in.Files)
	}
	// The bytes must arrive unchanged: no formatting, no appended main() call.
	if string(in.Files[0].Data) != body {
		t.Fatalf("data = %q, want %q", in.Files[0].Data, body)
	}
	if in.Dir != dir {
		t.Errorf("dir = %q, want %q", in.Dir, dir)
	}
}

// withGoBuildPackageFiles installs the same directory selection internal/agentos
// wires in: the Go toolchain's own, through go/build. The package under test
// cannot import internal/agentos (that is the dependency direction that keeps
// go/build out of cmd/bash), so the test supplies it.
func withGoBuildPackageFiles(t *testing.T) {
	t.Helper()
	previous := GoSourcePackageFiles
	GoSourcePackageFiles = func(dir string) ([]string, error) {
		pkg, err := build.Default.ImportDir(dir, 0)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(pkg.GoFiles))
		for _, name := range pkg.GoFiles {
			names = append(names, filepath.Join(dir, name))
		}
		return names, nil
	}
	t.Cleanup(func() { GoSourcePackageFiles = previous })
}

func TestCollectGoSourcesDirectoryRecipe(t *testing.T) {
	withGoBuildPackageFiles(t)
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("b.go", "package main\n")
	write("a.go", "package main\n")
	write("a_test.go", "package main\n")
	write("_ignored.go", "package main\n")
	write(".hidden.go", "package main\n")
	write("README.md", "not go\n")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	in, err := CollectGoSources(GoSourceResolution{Enabled: true}, dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range in.Files {
		got = append(got, filepath.Base(f.Name))
	}
	want := []string{"a.go", "b.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("files = %q, want %q", got, want)
	}
	if in.Dir != dir {
		t.Errorf("dir = %q, want %q", in.Dir, dir)
	}
}

// TestCollectGoSourcesDirectoryWithoutHook pins the refusal a build with no
// front end gives for a directory recipe: it names the missing front end and
// never falls back to a hand-rolled file list.
func TestCollectGoSourcesDirectoryWithoutHook(t *testing.T) {
	previous := GoSourcePackageFiles
	GoSourcePackageFiles = nil
	t.Cleanup(func() { GoSourcePackageFiles = previous })
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := CollectGoSources(GoSourceResolution{Enabled: true}, dir, "", nil)
	if err == nil || !errors.Is(err, ErrGoSourceUnavailable) {
		t.Fatalf("err = %v, want the front-end-unavailable refusal", err)
	}
}

func TestCollectGoSourcesEmptyDirectory(t *testing.T) {
	withGoBuildPackageFiles(t)
	_, err := CollectGoSources(GoSourceResolution{Enabled: true}, t.TempDir(), "", nil)
	if err == nil || !strings.Contains(err.Error(), "Go source files") {
		t.Fatalf("err = %v, want a no-Go-source-files refusal", err)
	}
}

func TestCollectGoSourcesExplicitFilesKeepOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z.go", "a.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{filepath.Join(dir, "z.go"), filepath.Join(dir, "a.go")}
	in, err := CollectGoSources(GoSourceResolution{Enabled: true, Files: files}, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Bashy does not sort; the front end owns file ordering (assumption A6).
	var got []string
	for _, f := range in.Files {
		got = append(got, f.Name)
	}
	if !slices.Equal(got, files) {
		t.Fatalf("files = %q, want %q", got, files)
	}
}

func TestCollectGoSourcesStdinAndCommand(t *testing.T) {
	in, err := CollectGoSources(GoSourceResolution{Enabled: true}, "", "", strings.NewReader("package main\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Files) != 1 || in.Files[0].Name != "-" {
		t.Fatalf("stdin files = %+v", in.Files)
	}
	in, err = CollectGoSources(GoSourceResolution{Enabled: true}, "", "package main\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Files) != 1 || in.Files[0].Name != "-c" {
		t.Fatalf("-c files = %+v", in.Files)
	}
}

// TestLoadGoSourceRefusesWithoutFrontEnd is the load-bearing negative: with no
// front end linked, a Go-source invocation must fail with a Go-source
// diagnostic. It must NOT succeed, and it must not reach the shell parser.
func TestLoadGoSourceRefusesWithoutFrontEnd(t *testing.T) {
	previous := GoSourceLoad
	GoSourceLoad = nil
	t.Cleanup(func() { GoSourceLoad = previous })

	in := GoSourceInput{Files: []GoSourceFile{{Name: "hello.go", Data: []byte("package main\n")}}}
	prog, err := LoadGoSource(in, GoSourceOptions{})
	if err == nil {
		t.Fatalf("want a refusal, got program %+v", prog)
	}
	if !errors.Is(err, ErrGoSourceUnavailable) {
		t.Fatalf("err = %v, want ErrGoSourceUnavailable", err)
	}
	if !strings.Contains(err.Error(), "--source=go") {
		t.Errorf("diagnostic %q does not name the selector it refused", err)
	}
}

// TestLoadGoSourceMalformedNeverReachesShell pins the sprint contract's item 6:
// malformed Go fails as Go. The hook stands in for the front end and returns a
// Go diagnostic; nothing in this package retries the bytes as shell.
func TestLoadGoSourceMalformedNeverReachesShell(t *testing.T) {
	previous := GoSourceLoad
	t.Cleanup(func() { GoSourceLoad = previous })
	wantErr := errors.New("bad.go:3:1: expected declaration, found 'if'")
	var seen []GoSourceFile
	GoSourceLoad = func(files []GoSourceFile, _ GoSourceOptions) (*GoSourceProgram, error) {
		seen = files
		return nil, wantErr
	}
	// Source that a shell parser would happily accept as commands.
	src := []byte("package main\n\nif true; then echo shell; fi\n")
	_, err := LoadGoSource(GoSourceInput{Files: []GoSourceFile{{Name: "bad.go", Data: src}}}, GoSourceOptions{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want the front end's own diagnostic", err)
	}
	if len(seen) != 1 || string(seen[0].Data) != string(src) {
		t.Fatalf("front end saw %+v, want the original bytes unchanged", seen)
	}
}

func TestGoSourceProgramSourceAt(t *testing.T) {
	// Two files laid out back to back, the way the front end concatenates a
	// package's position space (assumption A4).
	prog := &GoSourceProgram{Origins: []GoSourceOrigin{
		{Name: "a.go", Base: 0, Size: 10},
		{Name: "b.go", Base: 11, Size: 20},
	}}
	tests := []struct {
		offset   uint
		wantName string
		wantOff  uint
		wantOK   bool
	}{
		{offset: 0, wantName: "a.go", wantOff: 0, wantOK: true},
		{offset: 7, wantName: "a.go", wantOff: 7, wantOK: true},
		{offset: 15, wantName: "b.go", wantOff: 4, wantOK: true},
		{offset: 31, wantName: "b.go", wantOff: 20, wantOK: true},
		{offset: 99, wantOK: false},
	}
	for _, tc := range tests {
		name, off, ok := prog.SourceAt(syntax.NewPos(tc.offset, 0, 0))
		if ok != tc.wantOK || name != tc.wantName || off != tc.wantOff {
			t.Errorf("SourceAt(%d) = (%q, %d, %v), want (%q, %d, %v)",
				tc.offset, name, off, ok, tc.wantName, tc.wantOff, tc.wantOK)
		}
	}
	var nilProg *GoSourceProgram
	if _, _, ok := nilProg.SourceAt(syntax.NewPos(0, 0, 0)); ok {
		t.Error("a nil program must not resolve a position")
	}
}

func TestGoSourceFailurePrefixesOnlyBareErrors(t *testing.T) {
	// A refusal already carries the shell's name; sh's positioned diagnostics
	// must survive verbatim so their file:line:col still points at Go source.
	if got := goSourceDiagnosticText(goSourceErrorf("bashy: --check requires --source=go")); got != "bashy: --check requires --source=go" {
		t.Errorf("refusal text = %q", got)
	}
	if got := goSourceDiagnosticText(errors.New("hello.go:1:1: expected 'package'")); got != "bashy: hello.go:1:1: expected 'package'" {
		t.Errorf("diagnostic text = %q", got)
	}
}

// captureStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var sb strings.Builder
	if _, err := io.Copy(&sb, r); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func withGoSourceSelection(t *testing.T, res GoSourceResolution) {
	t.Helper()
	previous := startupGoSource
	startupGoSource = res
	t.Cleanup(func() { startupGoSource = previous })
}

func withGoSourceHook(t *testing.T, hook func([]GoSourceFile, GoSourceOptions) (*GoSourceProgram, error)) {
	t.Helper()
	previous := GoSourceLoad
	GoSourceLoad = hook
	t.Cleanup(func() { GoSourceLoad = previous })
}

func writeGoFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prog.go")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRunGoSourceInvocationRefusesWithoutFrontEnd exercises the whole
// invocation: an absent front end ends it with exit 2 and a diagnostic. This is
// what `bashy --bashpp --source=go prog.go` does in the default build today.
func TestRunGoSourceInvocationRefusesWithoutFrontEnd(t *testing.T) {
	path := writeGoFixture(t, "package main\n\nfunc main() {}\n")
	withGoSourceSelection(t, GoSourceResolution{Enabled: true, Files: []string{path}})
	withGoSourceHook(t, nil)

	var err error
	stderr := captureStderr(t, func() { err = runGoSourceInvocation() })
	if err == nil {
		t.Fatal("want a failure, got nil")
	}
	if got := exitStatusOf(t, err); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	if !strings.Contains(stderr, "not available in this build") {
		t.Errorf("stderr = %q", stderr)
	}
}

// TestRunGoSourceInvocationMalformedStaysGo is the sprint contract's item 6 at
// the invocation level: bytes that a shell would happily run are reported with
// the front end's Go diagnostic and nothing else.
func TestRunGoSourceInvocationMalformedStaysGo(t *testing.T) {
	const body = "package main\n\nif true; then echo shell-ran; fi\n"
	path := writeGoFixture(t, body)
	withGoSourceSelection(t, GoSourceResolution{Enabled: true, Files: []string{path}})
	const diagnostic = "prog.go:3:1: syntax error: non-declaration statement outside function body"
	withGoSourceHook(t, func(files []GoSourceFile, _ GoSourceOptions) (*GoSourceProgram, error) {
		if len(files) != 1 || string(files[0].Data) != body {
			t.Errorf("front end saw %+v, want the original bytes", files)
		}
		return nil, errors.New(diagnostic)
	})

	var err error
	stderr := captureStderr(t, func() { err = runGoSourceInvocation() })
	if got := exitStatusOf(t, err); got != 2 {
		t.Fatalf("exit = %d, want 2", got)
	}
	// Verbatim: a differential harness compares this against the Go toolchain.
	if strings.TrimSpace(stderr) != diagnostic {
		t.Errorf("stderr = %q, want %q verbatim", stderr, diagnostic)
	}
	if strings.Contains(stderr, "shell-ran") || strings.Contains(stderr, "command not found") {
		t.Errorf("shell dispatch leaked into a --source=go failure: %q", stderr)
	}
}

// TestRunGoSourceCheckExecutesNothing pins --check: the program is loaded
// WITHOUT entry calls and is never handed to a runner.
func TestRunGoSourceCheckExecutesNothing(t *testing.T) {
	path := writeGoFixture(t, "package main\n\nfunc main() { println(\"ran\") }\n")
	withGoSourceSelection(t, GoSourceResolution{Enabled: true, Check: true, Files: []string{path}})

	var sawRunMain bool
	var calls int
	withGoSourceHook(t, func(_ []GoSourceFile, opts GoSourceOptions) (*GoSourceProgram, error) {
		calls++
		sawRunMain = opts.RunMain
		// A program with statements: if --check ever ran it, they would fire.
		file, perr := syntax.NewParser().Parse(strings.NewReader("echo ran\n"), path)
		if perr != nil {
			t.Fatal(perr)
		}
		return &GoSourceProgram{File: file, Package: "main", Main: "main"}, nil
	})

	var err error
	stderr := captureStderr(t, func() { err = runGoSourceInvocation() })
	if err != nil {
		t.Fatalf("--check failed: %v (stderr %q)", err, stderr)
	}
	if calls != 1 {
		t.Errorf("front end called %d times, want 1", calls)
	}
	if sawRunMain {
		t.Error("--check must load with RunMain=false so the program carries no entry calls")
	}
}

func exitStatusOf(t *testing.T, err error) int {
	t.Helper()
	var es interp.ExitStatus
	if !errors.As(err, &es) {
		t.Fatalf("error %v is not an exit status", err)
	}
	return int(es)
}

// TestInvocationScannersAgreeOnValueFlags is review finding 5: the argv
// scanners each stop at the first token that does not look like an option, so
// one of them not knowing that `--source` takes a value made it read `go` as
// the script operand and drop every selector spelled behind it. The observed
// failure was `bashy --source go --bashpp x.go` exiting 2 with "flag provided
// but not defined: -bashpp".
func TestInvocationScannersAgreeOnValueFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantArgs []string
		wantLang string
		wantPP   bool
	}{
		{
			name:     "source before bashpp, separated",
			args:     []string{"bashy", "--source", "go", "--bashpp", "x.go"},
			wantArgs: []string{"bashy", "x.go"},
			wantLang: "go",
			wantPP:   true,
		},
		{
			name:     "source after bashpp, separated",
			args:     []string{"bashy", "--bashpp", "--source", "go", "x.go"},
			wantArgs: []string{"bashy", "x.go"},
			wantLang: "go",
			wantPP:   true,
		},
		{
			name:     "source before bashpp, joined",
			args:     []string{"bashy", "--source=go", "--bashpp", "x.go"},
			wantArgs: []string{"bashy", "x.go"},
			wantLang: "go",
			wantPP:   true,
		},
		{
			name:     "go-file value does not end the option scan",
			args:     []string{"bashy", "--go-file", "a.go", "--bashpp", "--source=go"},
			wantArgs: []string{"bashy"},
			wantLang: "go",
			wantPP:   true,
		},
		{
			name:     "another option's value does not end the scan either",
			args:     []string{"bashy", "--rcfile", "rc", "--source", "go", "--bashpp", "x.go"},
			wantArgs: []string{"bashy", "--rcfile", "rc", "x.go"},
			wantLang: "go",
			wantPP:   true,
		},
		{
			name:     "arguments after the operand are untouched",
			args:     []string{"bashy", "--source", "go", "--bashpp", "x.go", "--source", "sh", "-c", "arg"},
			wantArgs: []string{"bashy", "x.go", "--source", "sh", "-c", "arg"},
			wantLang: "go",
			wantPP:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Main's order: Go-source selectors first, then -c relocation and
			// the Bash++ strip.
			stripped, sel, err := stripGoSourceInvocationFlags(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			stripped = stripBashPPInvocationFlags(relocatePendingCommandFlag(stripped))
			if !slices.Equal(stripped, tc.wantArgs) {
				t.Errorf("argv = %q, want %q", stripped, tc.wantArgs)
			}
			if sel.Language != tc.wantLang {
				t.Errorf("--source = %q, want %q", sel.Language, tc.wantLang)
			}
			// ResolveBashPP reads the ORIGINAL argv, before any stripping, so
			// its scanner must step over the same values.
			enabled, seen := commandLineBashPP(tc.args)
			if enabled != tc.wantPP || !seen {
				t.Errorf("commandLineBashPP = (%v,%v), want (%v,true)", enabled, seen, tc.wantPP)
			}
		})
	}
}

// TestResolveGoSourceRefusesShellOnlyModes is review finding 4: --pretty-print
// and the --dump-strings family build the SHELL parser before any input
// dispatch, so `bashy --bashpp --source=go --pretty-print prog.go` reported a
// shell parse error for a valid Go program. They are refused, and the refusal
// is a Go-source refusal — no shell parser is constructed on the way to it.
func TestResolveGoSourceRefusesShellOnlyModes(t *testing.T) {
	for _, mode := range []string{"--pretty-print", "--dump-strings", "--dump-po-strings"} {
		t.Run(mode, func(t *testing.T) {
			_, err := ResolveGoSource(
				GoSourceSelection{Language: "go", LanguageSeen: true},
				GoSourceContext{Binary: BashPPBinaryBashy, BashPP: true, ShellOnlyMode: mode})
			if err == nil {
				t.Fatal("want a refusal")
			}
			if !IsGoSourceError(err) {
				t.Errorf("err = %v, want a Go-source refusal", err)
			}
			if !strings.Contains(err.Error(), mode) || !strings.Contains(err.Error(), "--source=go") {
				t.Errorf("err = %v, want it to name both %s and --source=go", err, mode)
			}
		})
	}
}

// TestStartupShellOnlyModeReportsSelection pins the mapping from the parsed
// flags to the refusal, so the refusal cannot silently stop firing when one of
// these modes is reached by a different spelling.
func TestStartupShellOnlyModeReportsSelection(t *testing.T) {
	for _, tc := range []struct {
		flag *bool
		want string
	}{
		{pretty, "--pretty-print"},
		{dumpStrs, "--dump-strings"},
		{dumpShort, "--dump-strings"},
		{dumpPO, "--dump-po-strings"},
	} {
		previous := *tc.flag
		*tc.flag = true
		got := startupShellOnlyMode()
		*tc.flag = previous
		if got != tc.want {
			t.Errorf("startupShellOnlyMode() = %q, want %q", got, tc.want)
		}
	}
	if got := startupShellOnlyMode(); got != "" {
		t.Errorf("startupShellOnlyMode() = %q with no mode selected, want \"\"", got)
	}
}

// TestRunGoSourceInvocationSkipsLogout is review finding 8: a Go program's
// semantics are Go's. A --login invocation must not append shell effects from
// ~/.bash_logout after main returns.
func TestRunGoSourceInvocationSkipsLogout(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, "logout-ran")
	logout := "printf x > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(home, ".bash_logout"), []byte(logout), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	path := writeGoFixture(t, "package main\n\nfunc main() {}\n")
	withGoSourceSelection(t, GoSourceResolution{Enabled: true, Files: []string{path}})
	withGoSourceHook(t, func(_ []GoSourceFile, _ GoSourceOptions) (*GoSourceProgram, error) {
		file, perr := syntax.NewParser().Parse(strings.NewReader(":\n"), path)
		if perr != nil {
			t.Fatal(perr)
		}
		return &GoSourceProgram{File: file, Package: "main", Main: "main"}, nil
	})
	previousLogin := *login
	*login = true
	t.Cleanup(func() { *login = previousLogin })

	if err := runGoSourceInvocation(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("~/.bash_logout ran after a --source=go program")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}

	// Positive control: the same fixture, run through the shell path's logout
	// wrapper, DOES source ~/.bash_logout. Without this the assertion above
	// would keep passing if the hook simply stopped working.
	r, err := newRunner()
	if err != nil {
		t.Fatal(err)
	}
	if err := runWithLoginLogout(r, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("control: the shell path did not run ~/.bash_logout: %v", err)
	}
}

// TestRunGoSourceInvocationParameters pins the CLI's half of argument
// handling: the operand is the program (it is $0, not $1), the selectors never
// become positional parameters, and the arguments after the operand are passed
// through in order. Evaluating os.Args inside the program is the interpreter's
// half and is measured separately.
func TestRunGoSourceInvocationParameters(t *testing.T) {
	path := writeGoFixture(t, "package main\n\nfunc main() {}\n")
	withGoSourceSelection(t, GoSourceResolution{Enabled: true})
	withGoSourceHook(t, func(_ []GoSourceFile, _ GoSourceOptions) (*GoSourceProgram, error) {
		// A shell fixture standing in for the lowered program: it reports
		// what the runner was actually given.
		file, perr := syntax.NewParser().Parse(strings.NewReader("printf '%s|%s\\n' \"$0\" \"$*\"\n"), path)
		if perr != nil {
			t.Fatal(perr)
		}
		return &GoSourceProgram{File: file, Package: "main", Main: "main"}, nil
	})

	previousArgs := os.Args
	os.Args = []string{"bashy", path, "one", "two"}
	t.Cleanup(func() { os.Args = previousArgs })
	if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = flag.CommandLine.Parse(nil) })

	stdout, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	previousStdout := os.Stdout
	os.Stdout = stdout
	runErr := runGoSourceInvocation()
	os.Stdout = previousStdout
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	want := path + "|one two\n"
	if string(data) != want {
		t.Errorf("runner saw %q, want %q", data, want)
	}
}

// TestRunGoSourceInvocationPassesModuleDir pins the seam the sh worker is
// landing as interp.GoSourceModuleDir: when it exists, the SOURCE's directory
// is handed to the runner, and the program's working directory is left alone.
func TestRunGoSourceInvocationPassesModuleDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prog.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	previousWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousWd) })

	withGoSourceSelection(t, GoSourceResolution{Enabled: true, Files: []string{path}})
	withGoSourceHook(t, func(_ []GoSourceFile, opts GoSourceOptions) (*GoSourceProgram, error) {
		if opts.Dir != dir {
			t.Errorf("load Dir = %q, want the SOURCE's directory %q", opts.Dir, dir)
		}
		file, perr := syntax.NewParser().Parse(strings.NewReader(":\n"), path)
		if perr != nil {
			t.Fatal(perr)
		}
		return &GoSourceProgram{File: file, Package: "main", Main: "main"}, nil
	})

	var seen string
	previousHook := GoSourceModuleDir
	GoSourceModuleDir = func(d string) interp.RunnerOption {
		seen = d
		return func(*interp.Runner) error { return nil }
	}
	t.Cleanup(func() { GoSourceModuleDir = previousHook })

	if err := runGoSourceInvocation(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if seen != dir {
		t.Errorf("module dir = %q, want %q", seen, dir)
	}
	// The working directory is NOT moved to the source: the harness runs from
	// an assets-only runtime directory and must keep it.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// macOS reports /private/var for /var, so compare resolved paths.
	wantWd, err := filepath.EvalSymlinks(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	if gotWd, err := filepath.EvalSymlinks(wd); err != nil {
		t.Fatal(err)
	} else if gotWd != wantWd {
		t.Errorf("working directory moved to %q, want %q", gotWd, wantWd)
	}
}
