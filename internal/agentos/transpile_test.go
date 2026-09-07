package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTranspileDispatchArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStderr string
	}{
		{
			name:       "missing all",
			args:       []string{},
			wantExit:   2,
			wantStderr: "transpile: missing INPUT\n",
		},
		{
			name:       "missing bashpp",
			args:       []string{"input.sh", "-o", "out.go"},
			wantExit:   2,
			wantStderr: "transpile: --bashpp is required\n",
		},
		{
			name:       "missing output",
			args:       []string{"--bashpp", "input.sh"},
			wantExit:   2,
			wantStderr: "transpile: missing -o OUTPUT.go\n",
		},
		{
			name:       "missing input",
			args:       []string{"--bashpp", "-o", "out.go"},
			wantExit:   2,
			wantStderr: "transpile: missing INPUT\n",
		},
		{
			name:       "file not found",
			args:       []string{"--bashpp", "does-not-exist.sh", "-o", "out.go"},
			wantExit:   2,
			wantStderr: "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			exitCode := dispatchTranspile(tt.args)

			w.Close()
			os.Stderr = oldStderr

			var buf bytes.Buffer
			io.Copy(&buf, r)
			stderr := buf.String()

			if exitCode != tt.wantExit {
				t.Errorf("got exit %d, want %d", exitCode, tt.wantExit)
			}
			if !strings.Contains(stderr, strings.TrimSpace(tt.wantStderr)) {
				t.Errorf("got stderr %q, want it to contain %q", stderr, tt.wantStderr)
			}
		})
	}
}

func TestTranspileInputEqualsOutputRejection(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "script.bpp")
	if err := os.WriteFile(file, []byte("echo hi\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	exitCode := dispatchTranspile([]string{"--bashpp", file, "-o", file})

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)
	stderr := buf.String()

	if exitCode != 2 {
		t.Errorf("got exit %d, want 2", exitCode)
	}
	if !strings.Contains(stderr, "cannot be the same file") {
		t.Errorf("got stderr %q, want path collision error", stderr)
	}
}

func TestTranspileSuccessAndExecution(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "test.bpp")
	outputFile := filepath.Join(dir, "main.go")
	mapFile := filepath.Join(dir, "main.go.map")

	shDir, err := filepath.Abs("../../../sh")
	if err != nil {
		t.Fatal(err)
	}
	goModContent := fmt.Sprintf("module testpkg\n\ngo 1.22\n\nrequire mvdan.cc/sh/v3 v3.0.0\nreplace mvdan.cc/sh/v3 => %s\n", shDir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatal(err)
	}

	script := `var x int = 42
func add(a int, b int) int {
	sum := a + b
	return sum
}
res := add(x, 10)
printf '%d\n' "$res"
`
	if err := os.WriteFile(inputFile, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	exitCode := dispatchTranspile([]string{"--bashpp", inputFile, "-o", outputFile})

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)
	stderr := buf.String()

	if exitCode != 0 {
		t.Fatalf("dispatchTranspile failed with exit %d, stderr: %s", exitCode, stderr)
	}

	outSource, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("could not read generated go output: %v", err)
	}
	if len(outSource) == 0 {
		t.Fatal("generated output file is empty")
	}

	// Verify source map artifact
	mapData, err := os.ReadFile(mapFile)
	if err != nil {
		t.Fatalf("could not read map artifact: %v", err)
	}
	var art struct {
		Origin   string `json:"origin"`
		GoDigest string `json:"go_digest"`
		Mappings []any  `json:"mappings"`
	}
	if err := json.Unmarshal(mapData, &art); err != nil {
		t.Fatalf("invalid map artifact JSON: %v", err)
	}
	if art.Origin != inputFile {
		t.Errorf("got map origin %q, want %q", art.Origin, inputFile)
	}
	if !strings.HasPrefix(art.GoDigest, "sha256:") {
		t.Errorf("got map go_digest %q, want sha256: prefix", art.GoDigest)
	}
	if len(art.Mappings) == 0 {
		t.Error("expected non-empty mappings array in map artifact")
	}

	// Tidy go.mod in test directory
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = dir
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v, output: %s", err, out)
	}

	// Test executing the generated Go program
	cmd := exec.Command("go", "run", outputFile)
	cmd.Dir = dir
	runOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run failed: %v\nOutput:\n%s\nGenerated Code:\n%s", err, runOut, outSource)
	}
	if strings.TrimSpace(string(runOut)) != "52" {
		t.Errorf("got program output %q, want 52", strings.TrimSpace(string(runOut)))
	}
}

func TestTranspileRepeatEmissionDeterministic(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "sample.bpp")
	outputFile1 := filepath.Join(dir, "out1.go")
	outputFile2 := filepath.Join(dir, "out2.go")

	script := `var base int = 100
echo hello
`
	if err := os.WriteFile(inputFile, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	exitCode1 := dispatchTranspile([]string{"--bashpp", inputFile, "-o", outputFile1})
	if exitCode1 != 0 {
		t.Fatalf("dispatchTranspile out1 failed: %d", exitCode1)
	}

	exitCode2 := dispatchTranspile([]string{"--bashpp", inputFile, "-o", outputFile2})
	if exitCode2 != 0 {
		t.Fatalf("dispatchTranspile out2 failed: %d", exitCode2)
	}

	src1, err := os.ReadFile(outputFile1)
	if err != nil {
		t.Fatal(err)
	}
	src2, err := os.ReadFile(outputFile2)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(src1, src2) {
		t.Errorf("emission mismatch across output paths:\nout1:\n%s\nout2:\n%s", src1, src2)
	}
}

func TestTranspileNegativeDiagnosticNoEmission(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "bad.bpp")
	outputFile := filepath.Join(dir, "out.go")

	// Pre-create output file to verify it is preserved untouched on error
	existingContent := []byte("// Existing content\n")
	if err := os.WriteFile(outputFile, existingContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Code with unsupported node / type error in lower compile
	badScript := `var x nonexistent_type = 123
`
	if err := os.WriteFile(inputFile, []byte(badScript), 0644); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	exitCode := dispatchTranspile([]string{"--bashpp", inputFile, "-o", outputFile})

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)
	stderr := buf.String()

	if exitCode != 2 {
		t.Errorf("got exit code %d, want 2", exitCode)
	}
	if !strings.Contains(stderr, "LOWER-") {
		t.Errorf("got stderr %q, want positioned LOWER- diagnostic", stderr)
	}

	// Verify existing output file was preserved
	currentContent, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(currentContent, existingContent) {
		t.Errorf("output file was modified on compile rejection: got %q, want %q", currentContent, existingContent)
	}
}
