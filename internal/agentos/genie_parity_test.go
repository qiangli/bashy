package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestGenieFixturePatchParity runs the shipped fixture adapter against two
// bashy binaries with the same deterministic OpenAI-compatible model. Set
// GENIE_PARITY_BEFORE_BIN and GENIE_PARITY_AFTER_BIN to run this opt-in gate.
func TestGenieFixturePatchParity(t *testing.T) {
	before, after := os.Getenv("GENIE_PARITY_BEFORE_BIN"), os.Getenv("GENIE_PARITY_AFTER_BIN")
	if before == "" || after == "" {
		t.Skip("set GENIE_PARITY_BEFORE_BIN and GENIE_PARITY_AFTER_BIN")
	}
	for _, bin := range []string{before, after} {
		if _, err := os.Stat(bin); err != nil {
			t.Fatal(err)
		}
	}
	outDir := os.Getenv("GENIE_PARITY_OUT")
	if outDir == "" {
		outDir = t.TempDir()
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	source, err := filepath.Abs(filepath.Join("..", "..", "..", "ycode", "examples", "genie"))
	if err != nil {
		t.Fatal(err)
	}
	bar := filepath.Join(source, "dist", "genie.bar")
	if _, err := os.Stat(bar); err != nil {
		t.Fatalf("build fixture bundle first: (cd %s && bashy dag -f dag.md package): %v", source, err)
	}
	var mu sync.Mutex
	count := 0
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		count++
		n := count
		mu.Unlock()
		_ = os.WriteFile(filepath.Join(outDir, fmt.Sprintf("model-request-%02d.json", n)), body, 0o600)
		w.Header().Set("Content-Type", "text/event-stream")
		if n%2 == 1 {
			args, _ := json.Marshal(map[string]string{"script": "sed -i 's/len(values) - 1/len(values)/' stats.py"})
			chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_edit", "type": "function", "function": map[string]string{"name": "bashy", "arguments": string(args)}}}}, "finish_reason": "tool_calls"}}})
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Fixed the mean denominator.\"},\"finish_reason\":\"stop\"}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer stub.Close()
	run := func(label, bin string) string {
		t.Helper()
		root := filepath.Join(outDir, label)
		repo := filepath.Join(root, "repo")
		if err := os.MkdirAll(repo, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"stats.py", ".gitignore", "tests/__init__.py", "tests/test_stats.py"} {
			data, err := os.ReadFile(filepath.Join(source, "fixture", "repo", name))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(repo, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		git := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = repo
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		git("init", "-q")
		git("add", "-A")
		git("-c", "user.name=genie", "-c", "user.email=genie@example.invalid", "commit", "-q", "-m", "fixture")
		cmd := exec.Command(bin, "genie", "solve", "Fix stats.mean so the tests pass")
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GENIE_BAR="+bar, "GENIE_EXTERNAL_MODEL=stub", "GENIE_MODEL_ID=stub-model", "GENIE_EXTERNAL_CONTEXT=32768", "OPENAI_BASE_URL="+stub.URL+"/v1", "OPENAI_API_KEY=stub", "GENIE_TOOLCHAINS=none", "GENIE_RUN_ID="+label, "GENIE_ARTIFACT_DIR="+filepath.Join(root, "artifacts"), "BASHY_HOME="+filepath.Join(root, "home"))
		output, err := cmd.CombinedOutput()
		_ = os.WriteFile(filepath.Join(outDir, label+".log"), output, 0o600)
		if err != nil {
			t.Fatalf("%s solve: %v\n%s", label, err, output)
		}
		var prediction string
		_ = filepath.Walk(filepath.Join(root, "artifacts"), func(path string, info os.FileInfo, err error) error {
			if err == nil && info != nil && info.Name() == "prediction.json" {
				prediction = path
			}
			return nil
		})
		if prediction == "" {
			t.Fatalf("%s: missing prediction.json", label)
		}
		data, err := os.ReadFile(prediction)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			ModelPatch string `json:"model_patch"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.ModelPatch, "+    return sum(values) / (len(values))") {
			t.Fatalf("%s: unexpected patch: %s", label, result.ModelPatch)
		}
		_ = os.WriteFile(filepath.Join(outDir, label+".patch"), []byte(result.ModelPatch), 0o600)
		return result.ModelPatch
	}
	oldPatch, newPatch := run("before", before), run("after", after)
	if !bytes.Equal([]byte(oldPatch), []byte(newPatch)) {
		t.Fatalf("model_patch bytes differ; inspect %s", outDir)
	}
	t.Logf("identical prediction.model_patch bytes saved in %s", outDir)
}
