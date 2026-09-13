package agentos

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/bashy/internal/cli"

	"mvdan.cc/sh/v3/gosource"
	"mvdan.cc/sh/v3/lower"
)

// Sprint 118, W1: `bashy transpile --bashpp --source=go`. The Go front end is
// linked into this package in the DEFAULT build (gosource.go), so these tests
// drive real Go source through it; a hook is substituted only where the point
// is a build that has no front end at all. See docs/plan-source-go-dispatch.md.

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readSourceMap(t *testing.T, path string) sourceMapArtifact {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var art sourceMapArtifact
	if err := json.Unmarshal(data, &art); err != nil {
		t.Fatal(err)
	}
	return art
}

func captureTranspileStderr(t *testing.T, args []string) (int, string) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	exit := dispatchTranspile(args)
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return exit, buf.String()
}

func captureTranspileOutput(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW
	exit := dispatchTranspile(args)
	outW.Close()
	errW.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	var out, stderr bytes.Buffer
	if _, err := io.Copy(&out, outR); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(&stderr, errR); err != nil {
		t.Fatal(err)
	}
	return exit, out.String(), stderr.String()
}

func TestTranspileGoLibrary(t *testing.T) {
	dir, out := filepath.Join("testdata", "sprint162", "library"), t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	x := filepath.Join(dir, "external_test.go")
	args := []string{"--bashpp", "--source=go", "--go-import-path", "example/library", "--go-library", out,
		"--go-file", a, "--go-file", b, "--go-xtest-file", x}
	exit, stdout, stderr := captureTranspileOutput(t, args)
	if exit != 0 {
		t.Fatalf("exit = %d, stderr %q", exit, stderr)
	}
	wantFor := make(map[string][]byte)
	for _, names := range [][]string{{a, b}, {x}} {
		var sources []gosource.Source
		for _, name := range names {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			sources = append(sources, gosource.Source{Name: name, Data: data})
		}
		program, err := gosource.Load(sources, gosource.Options{PreserveNativeInit: true, ImportPath: "example/library", Importer: lower.NewModuleImporter(dir)})
		if err != nil {
			t.Fatal(err)
		}
		want, err := lower.Compile(program.File, lower.Options{Package: program.Package, Library: true, Importer: program.Importer, Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		for _, generated := range want.Files {
			wantFor[filepath.Base(generated.Name)] = generated.Source
		}
	}
	for _, name := range []string{"a.go", "b.go", "external_test.go"} {
		if !strings.Contains(stdout, "library ") || !strings.Contains(stdout, " -> "+filepath.Join(out, name)) {
			t.Errorf("stdout %q does not report %s", stdout, name)
		}
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, wantFor[name]) {
			t.Errorf("%s differs from lower library output", name)
		}
		if _, err := os.Stat(filepath.Join(out, name+".map")); err != nil {
			t.Errorf("%s map: %v", name, err)
		}
	}
}

func TestTranspileGoLibraryRefusals(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.go")
	writeFile(t, src, "package p\n")
	notDir := filepath.Join(dir, "file")
	writeFile(t, notDir, "x")
	for _, tc := range []struct {
		name, want string
		args       []string
	}{
		{"source", "requires --source=go", []string{"--bashpp", "--go-library", dir, "--go-file", src}},
		{"flatten", "refuses --go-package", []string{"--bashpp", "--source=go", "--go-library", dir, "--go-import-path", "p", "--go-package", "q=" + src, "--go-file", src}},
		{"not directory", "path is not a directory", []string{"--bashpp", "--source=go", "--go-library", notDir, "--go-import-path", "p", "--go-file", src}},
		{"no go input", "requires Go input files", []string{"--bashpp", "--source=go", "--go-library", dir, "--go-import-path", "p", filepath.Join("testdata", "sprint162", "library", "negative-script.bpp")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exit, stderr := captureTranspileStderr(t, tc.args)
			if exit != 2 || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit %d stderr %q, want %q", exit, stderr, tc.want)
			}
		})
	}
	duplicate := filepath.Join(dir, "other", "a.go")
	writeFile(t, duplicate, "package p\n")
	exit, stderr := captureTranspileStderr(t, []string{"--bashpp", "--source=go", "--go-library", dir, "--go-import-path", "p", "--go-file", src, "--go-test-file", duplicate})
	if exit != 2 || !strings.Contains(stderr, "duplicate or unresolved library output basename") {
		t.Errorf("duplicate exit %d stderr %q", exit, stderr)
	}
}

func TestTranspileSourceSelectorRefusals(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "script.bpp")
	if err := os.WriteFile(src, []byte("echo hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.go")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "unknown language",
			args: []string{"--bashpp", "--source=rust", src, "-o", out},
			want: `transpile: --source: unknown input language "rust"`,
		},
		{
			name: "go-file needs go",
			args: []string{"--bashpp", "--go-file", "a.go", "-o", out},
			want: "transpile: --go-file requires --source=go",
		},
		{
			name: "go-file plus operand",
			args: []string{"--bashpp", "--source=go", "--go-file", "a.go", src, "-o", out},
			want: "transpile: --go-file cannot be combined with a file operand",
		},
		{
			name: "missing --source value",
			args: []string{"--bashpp", "--source"},
			want: "transpile: missing argument for --source",
		},
		{
			name: "missing --go-file value",
			args: []string{"--bashpp", "--source=go", "--go-file"},
			want: "transpile: missing argument for --go-file",
		},
		{
			// --bashpp stays required; --source=go does not imply it.
			name: "go still requires --bashpp",
			args: []string{"--source=go", src, "-o", out},
			want: "transpile: --bashpp is required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exit, stderr := captureTranspileStderr(t, tc.args)
			if exit != 2 {
				t.Errorf("exit = %d, want 2", exit)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.want)
			}
		})
	}
}

// TestTranspileGoWithoutFrontEnd is the honest current state: with no front end
// linked, a --source=go transpile refuses and writes nothing. It must never
// reparse the Go bytes as shell and emit an artifact anyway.
func TestTranspileGoWithoutFrontEnd(t *testing.T) {
	previous := cli.GoSourceLoad
	cli.GoSourceLoad = nil
	t.Cleanup(func() { cli.GoSourceLoad = previous })

	dir := t.TempDir()
	src := filepath.Join(dir, "hello.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.go")

	exit, stderr := captureTranspileStderr(t, []string{"--bashpp", "--source=go", src, "-o", out})
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if !strings.Contains(stderr, "not available in this build") {
		t.Errorf("stderr = %q, want the front-end-absent diagnostic", stderr)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("output %s exists after a refused transpile (err %v)", out, err)
	}
}

// TestTranspileGoDiagnosticIsVerbatim pins contract item 6: malformed Go in
// Go-source mode fails with the front end's own positioned diagnostic, and no
// shell parse is attempted on the same bytes.
func TestTranspileGoDiagnosticIsVerbatim(t *testing.T) {
	previous := cli.GoSourceLoad
	t.Cleanup(func() { cli.GoSourceLoad = previous })
	const diagnostic = "bad.go:3:1: syntax error: non-declaration statement outside function body"
	var saw [][]byte
	cli.GoSourceLoad = func(files []cli.GoSourceFile, _ cli.GoSourceOptions) (*cli.GoSourceProgram, error) {
		for _, f := range files {
			saw = append(saw, f.Data)
		}
		return nil, errFake(diagnostic)
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "bad.go")
	// Bytes a shell parser would accept as ordinary commands.
	body := "package main\n\nif true; then echo shell; fi\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.go")

	exit, stderr := captureTranspileStderr(t, []string{"--bashpp", "--source=go", src, "-o", out})
	if exit != 2 {
		t.Fatalf("exit = %d, want 2", exit)
	}
	if strings.TrimSpace(stderr) != diagnostic {
		t.Errorf("stderr = %q, want the Go diagnostic verbatim (%q)", stderr, diagnostic)
	}
	if len(saw) != 1 || string(saw[0]) != body {
		t.Errorf("front end saw %q, want the original bytes unchanged", saw)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("output %s exists after malformed Go", out)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

// TestTranspileGoSourceMapRecordsOriginalFiles proves the artifact side of
// contract item 6: every mapping names an ORIGINAL file and an offset into
// that file's own bytes, and the artifact records the front end and digests.
// TestTranspileGoSourceMapRecordsOriginalFiles drives the REAL front end over
// two real Go files: every mapping must name which original file it came from
// and an offset into that file's own bytes.
func TestTranspileGoSourceMapRecordsOriginalFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	writeFile(t, a, "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(greet()) }\n")
	writeFile(t, b, "package main\n\nfunc greet() string { return \"hi\" }\n")

	out := filepath.Join(dir, "out", "generated.go")
	mapFile := filepath.Join(dir, "out", "generated.go.map")
	exit, stderr := captureTranspileStderr(t,
		[]string{"--bashpp", "--source=go", "--go-file", a, "--go-file", b, "-o", out, "--map", mapFile})
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", exit, stderr)
	}

	art := readSourceMap(t, mapFile)
	if art.SourceKind != "go" {
		t.Errorf("source_kind = %q, want go", art.SourceKind)
	}
	if art.FrontEnd != gosource.Version {
		t.Errorf("front_end = %q, want %q", art.FrontEnd, gosource.Version)
	}
	if art.Origin != a {
		t.Errorf("origin = %q, want the first ORIGINAL file %q", art.Origin, a)
	}
	if len(art.Sources) != 2 || art.Sources[0].Name != a || art.Sources[1].Name != b {
		t.Fatalf("sources = %+v", art.Sources)
	}
	for _, src := range art.Sources {
		data, err := os.ReadFile(src.Name)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != src.SHA256 {
			t.Errorf("%s: recorded sha256 %s, file is %s", src.Name, src.SHA256, got)
		}
		if src.Size != uint(len(data)) {
			t.Errorf("%s: recorded size %d, file is %d bytes", src.Name, src.Size, len(data))
		}
	}
	if len(art.Mappings) == 0 {
		t.Fatal("no mappings recorded")
	}
	seen := map[string]bool{}
	for _, m := range art.Mappings {
		// Fail-closed: every Go-input mapping names an original file.
		if m.SourceFile == "" {
			t.Fatalf("mapping %+v has no source_file", m)
		}
		seen[m.SourceFile] = true
		data, err := os.ReadFile(m.SourceFile)
		if err != nil {
			t.Fatal(err)
		}
		if m.SourceFileOffset > uint(len(data)) {
			t.Errorf("mapping %+v points past the end of %s (%d bytes)", m, m.SourceFile, len(data))
		}
	}
	if !seen[a] || !seen[b] {
		t.Errorf("mappings named %v, want both original files", seen)
	}
}

// TestTranspileShellInputMapUnchanged guards the existing consumers: shell
// input must not grow any of the Go fields.
func TestTranspileShellInputMapUnchanged(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "script.bpp")
	if err := os.WriteFile(src, []byte("echo hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.go")
	mapFile := filepath.Join(dir, "out.go.map")

	for _, args := range [][]string{
		{"--bashpp", src, "-o", out, "--map", mapFile},
		{"--bashpp", "--source=sh", src, "-o", out, "--map", mapFile},
	} {
		exit, stderr := captureTranspileStderr(t, args)
		if exit != 0 {
			t.Fatalf("%q: exit = %d (stderr %q)", args, exit, stderr)
		}
		data, err := os.ReadFile(mapFile)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"source_kind", "front_end", "sources", "source_file"} {
			if bytes.Contains(data, []byte(`"`+field+`"`)) {
				t.Errorf("%q: shell map artifact carries %q", args, field)
			}
		}
	}
}
