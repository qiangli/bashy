package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTranspileRegisteredCLIDispatch(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		var args []string
		if err := json.Unmarshal([]byte(os.Getenv("TEST_OS_ARGS_JSON")), &args); err != nil {
			fmt.Fprintf(os.Stderr, "helper JSON unmarshal error: %v\n", err)
			os.Exit(2)
		}
		os.Args = args
		Dispatch()
		return
	}

	// Verify command registration metadata
	_, _, verbs := commandsCatalog()
	found := false
	for _, v := range verbs {
		if v == "transpile" {
			found = true
			break
		}
	}
	if !found {
		t.Error("transpile verb not found in commandsCatalog()")
	}

	rec := verbAtlasRecord("transpile", false)
	if rec.Synopsis == "" {
		t.Error("transpile verb has no synopsis in atlas record")
	}
	if len(rec.Caps) != 0 {
		t.Errorf("got transpile caps %v, want []", rec.Caps)
	}
	if len(rec.Effects) != 2 || rec.Effects[0] != "read" || rec.Effects[1] != "write" {
		t.Errorf("got transpile effects %v, want [read write]", rec.Effects)
	}

	// Subprocess helper invoking actual Dispatch() with os.Args, testing input/output paths with spaces
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "cli test input.bpp")
	outputFile := filepath.Join(dir, "cli test output.go")

	if err := os.WriteFile(inputFile, []byte("var x int = 10\necho hi\n"), 0644); err != nil {
		t.Fatal(err)
	}

	argsList := []string{"bashy", "transpile", "--bashpp", inputFile, "-o", outputFile}
	argsData, err := json.Marshal(argsList)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestTranspileRegisteredCLIDispatch")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"TEST_OS_ARGS_JSON="+string(argsData),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess Dispatch failed: %v, output: %s", err, out)
	}

	if _, err := os.Stat(outputFile); err != nil {
		t.Errorf("expected outputFile to exist after Dispatch(): %v", err)
	}
}

// Exercise the registered CLI in a separate process: input-file resolution must
// not depend on the launcher's cwd, while stdin uses that cwd deliberately.
func TestTranspileModuleInputDirectory(t *testing.T) {
	for _, kind := range []string{"absolute-file", "relative-file", "stdin"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			moduleDir := filepath.Join(root, "module")
			callerDir := filepath.Join(root, "unrelated")
			write := func(name, body string) {
				t.Helper()
				path := filepath.Join(moduleDir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write("go.mod", "module example.com/app\n\ngo 1.25\n")
			write("pkg/pkg.go", "package local\nimport \"fmt\"\nfunc Print() { fmt.Println(\"module\") }\n")
			source := "import \"example.com/app/pkg\"\nlocal.Print()\n"
			write("main.bpp", source)
			if err := os.MkdirAll(callerDir, 0700); err != nil {
				t.Fatal(err)
			}
			input := filepath.Join(moduleDir, "main.bpp")
			if kind == "relative-file" {
				var err error
				input, err = filepath.Rel(callerDir, input)
				if err != nil {
					t.Fatal(err)
				}
				// The CLI consumes shell-spelled paths. On Windows, filepath.Rel
				// uses backslashes, which a shell operand treats as literal
				// filename characters rather than directory separators.
				input = filepath.ToSlash(input)
			} else if kind == "stdin" {
				input = "-"
				callerDir = moduleDir
			}
			output := filepath.Join(moduleDir, ".artifact", "main.go")
			args, _ := json.Marshal([]string{"bashy", "transpile", "--bashpp", input, "-o", output})
			cmd := exec.Command(os.Args[0], "-test.run=^TestTranspileRegisteredCLIDispatch$")
			cmd.Dir = callerDir
			cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "TEST_OS_ARGS_JSON="+string(args), "BASHY_HINTS=off", "GOWORK=off", "GO111MODULE=on", "GOTOOLCHAIN=local", "GOPROXY=off")
			if kind == "stdin" {
				cmd.Stdin = strings.NewReader(source)
			}
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil || stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("CLI stdout=%q stderr=%q status=%v", stdout.String(), stderr.String(), err)
			}
			original, err := os.ReadFile(filepath.Join(moduleDir, "main.bpp"))
			if err != nil || string(original) != source {
				t.Fatalf("source changed: %v", err)
			}
			mapData, err := os.ReadFile(output + ".map")
			if err != nil {
				t.Fatal(err)
			}
			var mapping struct {
				Origin string `json:"origin"`
			}
			if err := json.Unmarshal(mapData, &mapping); err != nil || mapping.Origin != input {
				t.Fatalf("origin=%q, want exact argument %q; %v", mapping.Origin, input, err)
			}
			binary := filepath.Join(root, "program")
			if runtime.GOOS == "windows" {
				binary += ".exe"
			}
			cmd = exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, output)
			cmd.Dir = moduleDir
			cmd.Env = append(os.Environ(), "GOWORK=off", "GO111MODULE=on", "GOTOOLCHAIN=local")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("artifact build: %v: %s", err, out)
			}
			if err := os.Remove(filepath.Join(moduleDir, "main.bpp")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(output); err != nil {
				t.Fatal(err)
			}
			cmd = exec.Command(binary)
			cmd.Dir = callerDir
			cmd.Env = []string{"PATH=/no-tools"}
			stdout.Reset()
			stderr.Reset()
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil || stdout.String() != "module\n" || stderr.Len() != 0 {
				t.Fatalf("source-removed artifact stdout=%q stderr=%q status=%v", stdout.String(), stderr.String(), err)
			}
		})
	}
}
