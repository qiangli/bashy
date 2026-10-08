package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSourceRoute(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bashy")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	foreignSources := os.Getenv("BASHY_TEST_FOREIGN_SOURCES") == "1"
	buildArgs := []string{"build"}
	if !foreignSources {
		// Keep the Go-only gate fast; foreign sources need the full product's
		// AgentOS tool resolver to provision their language toolchains.
		buildArgs = append(buildArgs, "-tags", "bashy_core")
	}
	buildArgs = append(buildArgs, "-o", bin, "./cmd/bashy")
	build := exec.Command("go", buildArgs...)
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	dir := t.TempDir()
	goBody := "package main\nimport (\"fmt\"; \"os\")\nfunc main() { var in string; fmt.Fscan(os.Stdin, &in); fmt.Fprintln(os.Stderr, \"stderr\", in); fmt.Println(\"compiled\", os.Args[1], in); os.Exit(23) }\n"
	interpreted := "package main\nfunc main() { println(\"interpreted\") }\n"
	files := map[string]string{"x.go": goBody, "x.txt": goBody, "x": goBody, "x.bsh": interpreted, "interpret.go": "echo interpreted\n", "harness.go": interpreted, "x.c": "#include <stdio.h>\nint main(){fputs(\"c\", stdout);}\n", "x.cc": "#include <iostream>\nint main(){std::cout << \"cc\";}\n", "x.cpp": "#include <iostream>\nint main(){std::cout << \"cpp\";}\n", "x.cxx": "#include <iostream>\nint main(){std::cout << \"cxx\";}\n", "x.ts": "console.log('typescript')\n", "x.tsx": "console.log('tsx')\n", "x.js": "console.log('javascript', process.argv[2])\n", "x.mjs": "console.log('mjs')\n", "x.py": "import sys; print('python', sys.argv[1])\n", "x.rs": "fn main(){print!(\"rust\");}\n", "x.fs": "printfn \"fsharp\"\n", "x.fsx": "printfn \"fsharp-script\"\n"}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	moduleFiles := map[string]string{
		"module/go.mod":          "module example.test/route\n\ngo 1.27\n",
		"module/value/value.go":  "package value\nconst Text = \"module\"\n",
		"module/cmd/app/main.go": "package main\nimport (\"fmt\"; \"example.test/route/value\")\nfunc main(){fmt.Print(value.Text)}\n",
		"library.go":             "package library\nfunc Exported() int { return 1 }\n",
	}
	for name, body := range moduleFiles {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		args   []string
		output string
		code   int
	}{
		{"bsh Go content", []string{"x.bsh"}, "interpreted", 0},
		{"go extension", []string{"x.go", "arg"}, "compiled arg in", 23},
		{"go flag no op", []string{"--source=go", "x.go", "arg"}, "compiled arg in", 23},
		{"go flag txt", []string{"--source=go", "x.txt", "arg"}, "compiled arg in", 23},
		{"go flag no extension", []string{"--source=go", "x", "arg"}, "compiled arg in", 23},
		{"go module directory", []string{"--source=go", "module/cmd/app"}, "module", 0},
		{"non-main refusal", []string{"library.go"}, "library.go:1:1: Go source package library cannot run as a program; expose its exported functions from a ~~~go fence", 2},
		{"bashsharp override", []string{"--bashsharp", "interpret.go"}, "interpreted", 0},
		{"harness override", []string{"--bashpp", "--source=go", "harness.go"}, "interpreted", 0},
		{"c extension", []string{"x.c"}, "c", 0},
		{"cc extension", []string{"x.cc"}, "cc", 0},
		{"cpp extension", []string{"x.cpp"}, "cpp", 0},
		{"cxx extension", []string{"x.cxx"}, "cxx", 0},
		{"typescript extension", []string{"x.ts"}, "typescript", 0},
		{"tsx extension", []string{"x.tsx"}, "tsx", 0},
		{"javascript extension", []string{"x.js", "arg"}, "javascript arg", 0},
		{"module javascript extension", []string{"x.mjs"}, "mjs", 0},
		{"python extension", []string{"x.py", "arg"}, "python arg", 0},
		{"rust extension", []string{"x.rs"}, "rust", 0},
		{"flag beats python extension", []string{"--source=go", "x.py"}, "expected 'package'", 1},
		{"fsharp extension", []string{"x.fs"}, "fsharp", 0},
		{"fsharp script extension", []string{"x.fsx"}, "fsharp-script", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Foreign-language cases provision C/C++/Node/TS/Python/Rust/.NET
			// toolchains on first use: too heavy for every per-push CI job.
			// They run in the per-candidate evidence lanes (and on demand)
			// with BASHY_TEST_FOREIGN_SOURCES=1; the Go route cases always run.
			if foreignSourceCase(tc.args) && !foreignSources {
				t.Skip("set BASHY_TEST_FOREIGN_SOURCES=1 to provision foreign toolchains and run this case")
			}
			cmd := exec.Command(bin, tc.args...)
			cmd.Dir = dir
			cmd.Stdin = strings.NewReader("in\n")
			out, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				if e, ok := err.(*exec.ExitError); ok {
					code = e.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if code != tc.code || !strings.Contains(string(out), tc.output) {
				t.Fatalf("code=%d output=%q; want %d %q", code, out, tc.code, tc.output)
			}
			if tc.code == 23 && !strings.Contains(string(out), "stderr in") {
				t.Fatalf("stderr not passed through: %q", out)
			}
		})
	}
}

// foreignSourceCase reports whether a case runs a non-Go source file without
// --source=go overriding it (that override case stays a cheap Go refusal).
func foreignSourceCase(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "--source=") {
			return false
		}
	}
	for _, a := range args {
		switch filepath.Ext(a) {
		case ".c", ".cc", ".cpp", ".cxx", ".js", ".mjs", ".ts", ".tsx", ".py", ".rs", ".fs", ".fsx":
			return true
		}
	}
	return false
}
