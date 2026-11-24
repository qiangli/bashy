// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

package hostedrunner_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func exampleDir(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

func bashyBin(t *testing.T) string {
	t.Helper()
	if b := os.Getenv("BASHY_BIN"); b != "" {
		return b
	}
	b, err := exec.LookPath("bashy")
	if err != nil {
		t.Skip("bashy not found on PATH")
	}
	return b
}

func TestSkillAndDryRunJSON(t *testing.T) {
	bashy := bashyBin(t)
	dir := exampleDir(t)
	script := filepath.Join(dir, "hosted-runner.bsh")

	out, err := exec.Command(bashy, "--bashsharp", script, "--skill").CombinedOutput()
	if err != nil {
		t.Fatalf("--skill failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "name: hosted-runner") {
		t.Fatalf("--skill missing frontmatter:\n%s", out)
	}

	for _, verb := range []string{"build", "start", "stop", "status"} {
		t.Run(verb, func(t *testing.T) {
			cmd := exec.Command(bashy, "--bashsharp", script, "--dry-run", "--json", "--port", "22799", verb)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%s --dry-run --json failed: %v\nstderr=%s", verb, err, stderr.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatalf("%s invalid JSON %q: %v", verb, stdout.String(), err)
			}
			if payload["action"] != verb || payload["dry_run"] != true {
				t.Fatalf("%s unexpected JSON payload: %#v", verb, payload)
			}
		})
	}
}

func TestSignerVerifyContract(t *testing.T) {
	dir := exampleDir(t)
	signerBin := filepath.Join(t.TempDir(), "signer")
	if runtime.GOOS == "windows" {
		signerBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", signerBin, "./cmd/signer")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build signer: %v\n%s", err, out)
	}

	keyPath := filepath.Join(t.TempDir(), "runner.key")
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := os.WriteFile(keyPath, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	seen := map[string]bool{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := r.Header.Get("X-Bashy-Hosted-Identity")
		parts := strings.Split(hdr, ".")
		if len(parts) != 3 || parts[0] != "v1" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte("v1." + parts[1]))
		wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(parts[2]), []byte(wantSig)) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		rawID, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var id struct {
			V              string `json:"v"`
			Caller         string `json:"caller"`
			TaskID         string `json:"task_id"`
			IdempotencyKey string `json:"idempotency_key"`
			Iat            int64  `json:"iat"`
			Exp            int64  `json:"exp"`
		}
		if err := json.Unmarshal(rawID, &id); err != nil || id.V != "bashy-hosted-identity-v1" || id.Caller == "" || id.TaskID == "" || id.IdempotencyKey == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		now := time.Now().Unix()
		if id.Exp <= now || id.Exp-id.Iat > 300 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"tools": []map[string]string{{"name": "run_tool"}, {"name": "google"}, {"name": "trove"}}},
			})
		case "tools/call":
			mu.Lock()
			replayed := seen[id.IdempotencyKey]
			seen[id.IdempotencyKey] = true
			mu.Unlock()
			var sc any
			if replayed {
				sc = map[string]any{"state": "replayed", "task_id": id.TaskID}
			} else {
				sc = map[string]any{"stdout": "hello hosted\n", "stderr": "", "exit_code": 0}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"isError": replayed, "structuredContent": sc},
			})
		default:
			http.Error(w, "bad method", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	out, err := exec.Command(signerBin, "--url", srv.URL, "--key-file", keyPath, "--verify", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("signer --verify failed: %v\n%s", err, out)
	}
	var summary map[string]any
	if err := json.Unmarshal(out, &summary); err != nil {
		t.Fatalf("decode signer summary %q: %v", out, err)
	}
	if summary["ok"] != true || summary["unsigned_status"] != float64(401) || summary["replay_state"] != "replayed" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}
