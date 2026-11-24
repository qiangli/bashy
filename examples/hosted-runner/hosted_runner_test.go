// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

package hostedrunner_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
			args := []string{"--bashsharp", script, "--dry-run", "--json", "--port", "22799",
				"--with-fakes", "--cloudbox-url", "http://127.0.0.1:22752", "--allow", "cred", "--pids-limit", "8192", verb}
			cmd := exec.Command(bashy, args...)
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
			if verb == "build" && payload["with_fakes"] != true {
				t.Fatalf("build missing with_fakes=true: %#v", payload)
			}
			if verb == "start" && (payload["cloudbox_url"] != "http://127.0.0.1:22752" || payload["allow"] != "cred" || payload["pids_limit"] != float64(8192)) {
				t.Fatalf("start missing cloudbox_url/allow: %#v", payload)
			}
		})
	}
}

func TestFakeCLIContract(t *testing.T) {
	dir := exampleDir(t)
	fakeBin := filepath.Join(t.TempDir(), "fakecli")
	if runtime.GOOS == "windows" {
		fakeBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", fakeBin, "./cmd/fakecli")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fakecli: %v\n%s", err, out)
	}

	credFile := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(credFile, []byte(`{"refresh_token":"fake-refresh"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	gmailCmd := exec.Command(fakeBin, "--as", "gws", "gmail", "users", "messages", "list", "--params", `{"userId":"me"}`)
	gmailCmd.Env = append(os.Environ(), "GWS_ACCOUNT=bench@example.test", "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE="+credFile)
	gmailOut, err := gmailCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fake gws gmail failed: %v\n%s", err, gmailOut)
	}
	if !strings.Contains(string(gmailOut), `"msg-1"`) {
		t.Fatalf("unexpected gmail output: %s", gmailOut)
	}

	driveCmd := exec.Command(fakeBin, "--as", "gws", "drive", "files", "list", "--params", `{"q":"'project' in parents and trashed = false"}`)
	driveCmd.Env = append(os.Environ(), "GWS_ACCOUNT=bench@example.test", "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE="+credFile)
	driveOut, err := driveCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fake gws drive failed: %v\n%s", err, driveOut)
	}
	if !strings.Contains(string(driveOut), `"O'Brien.txt"`) {
		t.Fatalf("unexpected drive output: %s", driveOut)
	}

	ghOut, err := exec.Command(fakeBin, "--as", "gh", "pr", "list").CombinedOutput()
	if err != nil {
		t.Fatalf("fake gh pr list failed: %v\n%s", err, ghOut)
	}
	if !strings.Contains(string(ghOut), `"number":42`) {
		t.Fatalf("unexpected gh output: %s", ghOut)
	}
}

type verifiedIdentity struct {
	V              string `json:"v"`
	Caller         string `json:"caller"`
	TaskID         string `json:"task_id"`
	IdempotencyKey string `json:"idempotency_key"`
	SecretsToken   string `json:"secrets_token"`
	Approval       *struct {
		ApprovalID string   `json:"approval_id"`
		Digest     string   `json:"digest"`
		Effects    []string `json:"effects"`
	} `json:"approval"`
	Iat int64 `json:"iat"`
	Exp int64 `json:"exp"`
}

func verifyTestHeader(hdr, key string) (verifiedIdentity, bool) {
	parts := strings.Split(hdr, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return verifiedIdentity{}, false
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("v1." + parts[1]))
	wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(wantSig)) {
		return verifiedIdentity{}, false
	}
	rawID, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return verifiedIdentity{}, false
	}
	var id verifiedIdentity
	if err := json.Unmarshal(rawID, &id); err != nil || id.V != "bashy-hosted-identity-v1" || id.Caller == "" || id.TaskID == "" || id.IdempotencyKey == "" {
		return verifiedIdentity{}, false
	}
	now := time.Now().Unix()
	if id.Exp <= now || id.Exp-id.Iat > 300 {
		return verifiedIdentity{}, false
	}
	return id, true
}

func expectedRunToolDigest(id verifiedIdentity, tool, name string, args []string) string {
	parts := []string{"bashy-hosted-call-v1", id.Caller, id.TaskID, tool, name, "", "", fmt.Sprintf("%d", len(args))}
	parts = append(parts, args...)
	parts = append(parts, "0")
	sum := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(sum, "%d:%s;", len(p), p)
	}
	return fmt.Sprintf("%x", sum.Sum(nil))
}

func buildSignerBin(t *testing.T, dir string) string {
	t.Helper()
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
	return signerBin
}

func writeTestKey(t *testing.T) (string, string) {
	t.Helper()
	keyPath := filepath.Join(t.TempDir(), "runner.key")
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := os.WriteFile(keyPath, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return keyPath, key
}

func TestSignerVerifyContract(t *testing.T) {
	dir := exampleDir(t)
	signerBin := buildSignerBin(t, dir)
	keyPath, key := writeTestKey(t)

	var mu sync.Mutex
	seen := map[string]bool{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := verifyTestHeader(r.Header.Get("X-Bashy-Hosted-Identity"), key)
		if !ok {
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

func TestSignerConcurrentLoadAndApproval(t *testing.T) {
	dir := exampleDir(t)
	signerBin := buildSignerBin(t, dir)
	keyPath, key := writeTestKey(t)

	var mu sync.Mutex
	seenKeys := map[string]bool{}
	seenCallers := map[string]bool{}
	toolCounts := map[string]int{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := verifyTestHeader(r.Header.Get("X-Bashy-Hosted-Identity"), key)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
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
		case "tools/call":
			mu.Lock()
			if seenKeys[id.IdempotencyKey] {
				mu.Unlock()
				http.Error(w, "duplicate idempotency key", http.StatusConflict)
				return
			}
			seenKeys[id.IdempotencyKey] = true
			seenCallers[id.Caller] = true
			toolCounts[req.Params.Name]++
			mu.Unlock()

			switch req.Params.Name {
			case "google", "trove":
				if id.SecretsToken == "" {
					http.Error(w, "missing secrets_token", http.StatusBadRequest)
					return
				}
			case "run_tool":
				var in struct {
					Name string   `json:"name"`
					Args []string `json:"args"`
				}
				_ = json.Unmarshal(req.Params.Arguments, &in)
				if in.Name == "gh" {
					wantDig := expectedRunToolDigest(id, "run_tool", "gh", in.Args)
					if id.Approval == nil || id.Approval.Digest != wantDig || len(id.Approval.Effects) == 0 || id.Approval.Effects[0] != "cred" {
						http.Error(w, "missing or invalid cred approval", http.StatusForbidden)
						return
					}
				}
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{
					"isError": false,
					"structuredContent": map[string]any{
						"stdout": "ok\n", "stderr": "", "exit_code": 0,
					},
				},
			})
		default:
			http.Error(w, "bad method", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	out, err := exec.Command(
		signerBin,
		"--url", srv.URL,
		"--key-file", keyPath,
		"--load",
		"--burst", "16",
		"--concurrency", "4",
		"--mix", "echo,gmail,trove,gh",
		"--fake-vault-listen", "127.0.0.1:0",
		"--json",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("signer --load failed: %v\n%s", err, out)
	}
	var summary struct {
		Mode        string         `json:"mode"`
		Total       int            `json:"total"`
		OK          int            `json:"ok"`
		Errors      int            `json:"errors"`
		ElapsedMs   float64        `json:"elapsed_ms"`
		AchievedRPS float64        `json:"achieved_rps"`
		ByTool      map[string]any `json:"by_tool"`
	}
	if err := json.Unmarshal(out, &summary); err != nil {
		t.Fatalf("decode load summary %q: %v", out, err)
	}
	if summary.Mode != "burst" || summary.Total != 16 || summary.OK != 16 || summary.Errors != 0 {
		t.Fatalf("unexpected load summary: %+v", summary)
	}
	if len(seenKeys) != 16 || len(seenCallers) != 16 {
		t.Fatalf("expected 16 unique keys and callers, got keys=%d callers=%d", len(seenKeys), len(seenCallers))
	}
	if toolCounts["run_tool"] != 8 || toolCounts["google"] != 4 || toolCounts["trove"] != 4 {
		t.Fatalf("unexpected tool counts: %v", toolCounts)
	}
	for _, k := range []string{"echo", "gmail", "trove", "gh"} {
		if _, ok := summary.ByTool[k]; !ok {
			t.Fatalf("missing tool %q in summary.ByTool: %v", k, summary.ByTool)
		}
	}
}

func TestSignerOpenLoopDrop(t *testing.T) {
	dir := exampleDir(t)
	signerBin := buildSignerBin(t, dir)
	keyPath, key := writeTestKey(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := verifyTestHeader(r.Header.Get("X-Bashy-Hosted-Identity"), key); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			time.Sleep(150 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{
					"isError":           false,
					"structuredContent": map[string]any{"stdout": "ok\n", "stderr": "", "exit_code": 0},
				},
			})
		}
	}))
	defer srv.Close()

	cmd := exec.Command(
		signerBin,
		"--url", srv.URL,
		"--key-file", keyPath,
		"--load", "--open-loop",
		"--rate", "40", "--duration", "500ms",
		"--concurrency", "1", "--queue-cap", "1",
		"--mix", "echo", "--json",
	)
	out, _ := cmd.CombinedOutput()
	var summary struct {
		Mode         string         `json:"mode"`
		Total        int            `json:"total"`
		OK           int            `json:"ok"`
		Errors       int            `json:"errors"`
		HTTPStatuses map[string]int `json:"http_statuses"`
	}
	if err := json.Unmarshal(out, &summary); err != nil {
		t.Fatalf("decode open-loop summary %q: %v", out, err)
	}
	if summary.Mode != "sustained" || summary.Total != 20 || summary.OK == 0 || summary.HTTPStatuses["503"] == 0 {
		t.Fatalf("unexpected open-loop summary: %+v", summary)
	}
}
