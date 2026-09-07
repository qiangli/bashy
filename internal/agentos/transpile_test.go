package agentos

import (
	"bytes"
	"crypto/sha256"
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

func TestTranspilePathCollisionsAndAliases(t *testing.T) {
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "script.bpp")
	if err := os.WriteFile(srcFile, []byte("echo hi\n"), 0644); err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(dir, "out.go")

	// 1. input == output
	t.Run("input_equals_output", func(t *testing.T) {
		exit := dispatchTranspile([]string{"--bashpp", srcFile, "-o", srcFile})
		if exit != 2 {
			t.Errorf("got exit %d, want 2", exit)
		}
	})

	// 2. map == input
	t.Run("map_equals_input", func(t *testing.T) {
		exit := dispatchTranspile([]string{"--bashpp", srcFile, "-o", outFile, "--map", srcFile})
		if exit != 2 {
			t.Errorf("got exit %d, want 2", exit)
		}
	})

	// 3. map == output
	t.Run("map_equals_output", func(t *testing.T) {
		exit := dispatchTranspile([]string{"--bashpp", srcFile, "-o", outFile, "--map", outFile})
		if exit != 2 {
			t.Errorf("got exit %d, want 2", exit)
		}
	})

	// 4. symlink collision
	symFile := filepath.Join(dir, "symlink.bpp")
	if err := os.Symlink(srcFile, symFile); err == nil {
		t.Run("symlink_collision", func(t *testing.T) {
			exit := dispatchTranspile([]string{"--bashpp", srcFile, "-o", symFile})
			if exit != 2 {
				t.Errorf("got exit %d for symlink collision, want 2", exit)
			}
		})
	}

	// 5. hardlink collision
	hardFile := filepath.Join(dir, "hardlink.bpp")
	if err := os.Link(srcFile, hardFile); err == nil {
		t.Run("hardlink_collision", func(t *testing.T) {
			exit := dispatchTranspile([]string{"--bashpp", srcFile, "-o", hardFile})
			if exit != 2 {
				t.Errorf("got exit %d for hardlink collision, want 2", exit)
			}
		})
	}

	// 6. Destination directory collisions
	outDir := filepath.Join(dir, "out_dir")
	if err := os.Mkdir(outDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("output_is_directory", func(t *testing.T) {
		exit := dispatchTranspile([]string{"--bashpp", srcFile, "-o", outDir})
		if exit != 2 {
			t.Errorf("got exit %d when output is directory, want 2", exit)
		}
	})

	t.Run("map_is_directory", func(t *testing.T) {
		exit := dispatchTranspile([]string{"--bashpp", srcFile, "-o", outFile, "--map", outDir})
		if exit != 2 {
			t.Errorf("got exit %d when map is directory, want 2", exit)
		}
	})
}

func TestTranspileDashInputPath(t *testing.T) {
	dir := t.TempDir()
	dashFile := filepath.Join(dir, "-script.bpp")
	outFile := filepath.Join(dir, "out.go")

	if err := os.WriteFile(dashFile, []byte("echo hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	exitCode := dispatchTranspile([]string{"--bashpp", "-o", outFile, "--", dashFile})
	if exitCode != 0 {
		t.Fatalf("dispatchTranspile with dash input path failed with exit %d", exitCode)
	}

	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
}

func TestTranspileGoBuildAndStandaloneExecute(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "test.bpp")
	outputFile := filepath.Join(dir, "main.go")
	mapFile := filepath.Join(dir, "main.go.map")
	binFile := filepath.Join(dir, "app")

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

	// Verify exact digest matching bytes in source map artifact
	mapData, err := os.ReadFile(mapFile)
	if err != nil {
		t.Fatalf("could not read map artifact: %v", err)
	}
	var art struct {
		SchemaVersion string     `json:"schema_version"`
		Origin        string     `json:"origin"`
		GoDigest      string     `json:"go_digest"`
		Mappings      []mapEntry `json:"mappings"`
	}
	if err := json.Unmarshal(mapData, &art); err != nil {
		t.Fatalf("invalid map artifact JSON: %v", err)
	}

	if art.SchemaVersion != sourceMapSchemaVersion {
		t.Errorf("got schema_version %q, want %q", art.SchemaVersion, sourceMapSchemaVersion)
	}
	if art.Origin != inputFile {
		t.Errorf("got map origin %q, want %q", art.Origin, inputFile)
	}
	expectedDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(outSource))
	if art.GoDigest != expectedDigest {
		t.Errorf("got go_digest %q, want exact matching %q", art.GoDigest, expectedDigest)
	}
	if len(art.Mappings) == 0 {
		t.Fatal("expected non-empty mappings array in map artifact")
	}

	// Assert explicit mapping fields
	firstMap := art.Mappings[0]
	if firstMap.GoLine <= 0 || firstMap.GoCol <= 0 {
		t.Errorf("invalid go position in map entry: %+v", firstMap)
	}
	if firstMap.SourceLine <= 0 || firstMap.Node == "" {
		t.Errorf("invalid source line or node in map entry: %+v", firstMap)
	}

	// Tidy go.mod in test directory
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = dir
	if out, err := tidyCmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v, output: %s", err, out)
	}

	// Actual go build binary
	buildCmd := exec.Command("go", "build", "-o", binFile, outputFile)
	buildCmd.Dir = dir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v, output: %s", err, out)
	}

	// Remove source script (.bpp) AND generated Go source file (.go) to prove standalone artifact execution!
	if err := os.Remove(inputFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(outputFile); err != nil {
		t.Fatal(err)
	}

	// Run binary with clean, shellfree PATH
	execCmd := exec.Command(binFile)
	execCmd.Dir = dir
	execCmd.Env = []string{"PATH=/bin:/usr/bin"}
	runOut, err := execCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("standalone binary execution failed: %v\nOutput:\n%s", err, runOut)
	}
	if strings.TrimSpace(string(runOut)) != "52" {
		t.Errorf("got program output %q, want 52", strings.TrimSpace(string(runOut)))
	}
}

func TestTranspileRegisteredCLIDispatch(t *testing.T) {
	// Verify command registration
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

	// Test registered CLI dispatch path
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "cli_test.bpp")
	outputFile := filepath.Join(dir, "cli_out.go")

	if err := os.WriteFile(inputFile, []byte("var x int = 10\necho hi\n"), 0644); err != nil {
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

	if exitCode != 0 {
		t.Errorf("dispatchTranspile failed via registered path: %d", exitCode)
	}
	if _, err := os.Stat(outputFile); err != nil {
		t.Errorf("expected outputFile to exist after registered dispatch: %v", err)
	}
}

func TestTranspileAtomicRollbackOnSecondRenameFailure(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.go")
	mapFile := filepath.Join(dir, "bad_map_dir") // directory path will cause rename to fail

	existingOut := []byte("// Old output content\n")
	if err := os.WriteFile(outFile, existingOut, 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(mapFile, 0755); err != nil {
		t.Fatal(err)
	}

	outData := []byte("// New Go source\n")
	mapData := []byte("{}")

	exit := writeOutputsAtomic(outFile, outData, mapFile, mapData)
	if exit != 2 {
		t.Errorf("got exit %d when map rename fails, want 2", exit)
	}

	// Verify old output file was restored via rollback
	restored, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("could not read restored output file: %v", err)
	}
	if !bytes.Equal(restored, existingOut) {
		t.Errorf("rollback failed: got %q, want %q", restored, existingOut)
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
	mapFile := filepath.Join(dir, "out.go.map")

	// Pre-create output and map file to verify both are preserved untouched on error
	existingContent := []byte("// Existing content\n")
	existingMap := []byte("// Existing map\n")
	if err := os.WriteFile(outputFile, existingContent, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mapFile, existingMap, 0644); err != nil {
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

	// Verify existing map file was preserved
	currentMap, err := os.ReadFile(mapFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(currentMap, existingMap) {
		t.Errorf("map file was modified on compile rejection: got %q, want %q", currentMap, existingMap)
	}
}
