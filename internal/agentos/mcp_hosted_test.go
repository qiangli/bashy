// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/yoke/pkg/fleet"
)

const mcpHostedFixtureKey = "unit-test-hosted-runner-key"

func mcpHostedFixtureIdentity(key string) mcpHostedIdentity {
	now := time.Now()
	return mcpHostedIdentity{Schema: mcpHostedSchema, Caller: "caller@example.test", TaskID: "task-1",
		IdempotencyKey: key, IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix()}
}

func mcpHostedToken(t *testing.T, id mcpHostedIdentity, key string) string {
	t.Helper()
	data, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	return mcpHostedTokenVersion + "." + payload + "." + mcpHostedSign([]byte(key), payload)
}

// mcpHostedFixture is a runner whose key, scratch root and audit log live in
// the test's own directories and whose child process is this test binary
// running the real bashy entry point.
func mcpHostedFixture(t *testing.T, timeout time.Duration, maxOutput int) (*mcpHosted, string) {
	t.Helper()
	ringDir(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "runner.key")
	if err := os.WriteFile(keyPath, []byte(mcpHostedFixtureKey+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(dir, "scratch")
	if err := os.Mkdir(scratch, 0700); err != nil {
		t.Fatal(err)
	}
	auditLog := filepath.Join(dir, "audit.jsonl")
	t.Setenv(mcpHostedKeyEnv, keyPath)
	t.Setenv(mcpHostedScratchEnv, scratch)
	t.Setenv("BASHY_AUDIT", auditLog)
	t.Setenv("BASHY_CLOUDBOX_URL", "http://127.0.0.1:1")
	h, err := newMCPHosted("", "default", "test", timeout, maxOutput)
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	h.self, h.childEnv = self, []string{adapterInprocEnv + "=1"}
	return h, auditLog
}

func TestMCPHostedIdentity(t *testing.T) {
	h, _ := mcpHostedFixture(t, time.Minute, defaultMCPMaxOutput)
	now := time.Now()
	for _, tc := range []string{"valid", "unsigned", "wrong-key", "tampered", "expired", "long-lived", "future", "no-caller",
		"no-idempotency-key", "wrong-schema", "wrong-version", "two-headers", "non-loopback", "public-key-file", "short-key", "missing-key"} {
		t.Run(tc, func(t *testing.T) {
			if err := os.WriteFile(h.keyPath, []byte(mcpHostedFixtureKey), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(h.keyPath, 0600); err != nil {
				t.Fatal(err)
			}
			id := mcpHostedFixtureIdentity("key-" + tc)
			r := httptest.NewRequest("POST", "http://localhost/mcp", nil)
			r.RemoteAddr = "127.0.0.1:1234"
			key := mcpHostedFixtureKey
			switch tc {
			case "wrong-key":
				key = "another-hosted-runner-key"
			case "expired":
				id.IssuedAt, id.ExpiresAt = now.Add(-2*time.Minute).Unix(), now.Add(-time.Minute).Unix()
			case "long-lived":
				id.ExpiresAt = now.Add(time.Hour).Unix()
			case "future":
				id.IssuedAt, id.ExpiresAt = now.Add(10*time.Minute).Unix(), now.Add(11*time.Minute).Unix()
			case "no-caller":
				id.Caller = " "
			case "no-idempotency-key":
				id.IdempotencyKey = ""
			case "wrong-schema":
				id.Schema = "bashy-hosted-identity-v0"
			case "non-loopback":
				r.RemoteAddr = "192.0.2.1:1234"
			case "public-key-file":
				if runtime.GOOS == "windows" {
					t.Skip("file modes are not enforced")
				}
				if err := os.Chmod(h.keyPath, 0644); err != nil {
					t.Fatal(err)
				}
			case "short-key":
				key = "short"
				if err := os.WriteFile(h.keyPath, []byte(key), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-key":
				if err := os.Remove(h.keyPath); err != nil {
					t.Fatal(err)
				}
			}
			token := mcpHostedToken(t, id, key)
			switch tc {
			case "unsigned":
				token = token[:strings.LastIndex(token, ".")+1]
			case "tampered":
				other := id
				other.Caller = "other@example.test"
				forged := strings.Split(mcpHostedToken(t, other, "another-hosted-runner-key"), ".")
				token = forged[0] + "." + forged[1] + "." + strings.Split(token, ".")[2]
			case "wrong-version":
				token = "v2" + strings.TrimPrefix(token, mcpHostedTokenVersion)
			}
			if tc != "unsigned" || token != "" {
				r.Header.Set(mcpHostedHeader, token)
			}
			if tc == "two-headers" {
				r.Header.Add(mcpHostedHeader, token)
			}
			got, ok := h.identity(r)
			if ok != (tc == "valid") {
				t.Fatalf("accepted = %v", ok)
			}
			if ok && got != id {
				t.Fatalf("identity = %+v, want %+v", got, id)
			}
		})
	}
	t.Run("no-header", func(t *testing.T) {
		r := httptest.NewRequest("POST", "http://localhost/mcp", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		if _, ok := h.identity(r); ok {
			t.Fatal("a request without an identity was accepted")
		}
	})
}

// mcpHostedTransport signs every request of a client with its current token.
type mcpHostedTransport struct {
	mu    sync.Mutex
	token string
}

func (tr *mcpHostedTransport) set(token string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.token = token
}

func (tr *mcpHostedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	tr.mu.Lock()
	r.Header.Set(mcpHostedHeader, tr.token)
	tr.mu.Unlock()
	return http.DefaultTransport.RoundTrip(r)
}

type mcpHostedReply struct {
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	ExitCode int      `json:"exit_code"`
	State    string   `json:"state"`
	Effects  []string `json:"effects"`
}

func mcpHostedCallTool(t *testing.T, ctx context.Context, cs *mcpsdk.ClientSession, name string, args map[string]any) mcpHostedReply {
	t.Helper()
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var reply mcpHostedReply
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		t.Fatalf("structured content %s: %v", data, err)
	}
	return reply
}

func mcpHostedScratchEmpty(t *testing.T, h *mcpHosted) {
	t.Helper()
	entries, err := os.ReadDir(h.scratch)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("scratch root still holds %d entries after the call", len(entries))
	}
}

func TestMCPHostedServe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the child process of this test is the unix test binary")
	}
	h, auditLog := mcpHostedFixture(t, time.Minute, defaultMCPMaxOutput)
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Unsigned and expired requests never reach a server.
	expired := mcpHostedFixtureIdentity("expired")
	expired.IssuedAt, expired.ExpiresAt = time.Now().Add(-2*time.Minute).Unix(), time.Now().Add(-time.Minute).Unix()
	for name, token := range map[string]string{"unsigned": "", "expired": mcpHostedToken(t, expired, mcpHostedFixtureKey),
		"wrong-key": mcpHostedToken(t, mcpHostedFixtureIdentity("k"), "another-hosted-runner-key")} {
		req, _ := http.NewRequestWithContext(ctx, "POST", server.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			req.Header.Set(mcpHostedHeader, token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s request: status %d, want 401", name, resp.StatusCode)
		}
	}

	transport := &mcpHostedTransport{}
	first := mcpHostedFixtureIdentity("call-1")
	transport.set(mcpHostedToken(t, first, mcpHostedFixtureKey))
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx,
		&mcpsdk.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: transport}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	// Shell sessions and the in-process state tools are not served.
	names := map[string]bool{}
	params := &mcpsdk.ListToolsParams{}
	for {
		page, err := cs.ListTools(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range page.Tools {
			names[tool.Name] = true
		}
		if page.NextCursor == "" {
			break
		}
		params.Cursor = page.NextCursor
	}
	for _, absent := range append([]string{"shell_open", "shell_exec", "shell_close"}, mcpHostedInProcess()...) {
		if names[absent] {
			t.Errorf("tools/list serves %s", absent)
		}
	}
	if !names["run_tool"] {
		t.Fatal("tools/list does not serve run_tool")
	}

	// A destroy verb answers gated, executes nothing and does not spend the identity.
	gated := mcpHostedCallTool(t, ctx, cs, "run_tool", map[string]any{"name": "rm", "args": []string{"-rf", "anything"}})
	if gated.State != "gated" || len(gated.Effects) == 0 || gated.Effects[0] != "destroy" {
		t.Fatalf("destroy call = %+v, want state gated with effect destroy", gated)
	}

	// run_tool echo round-trips through a child with the call's own home.
	echo := mcpHostedCallTool(t, ctx, cs, "run_tool", map[string]any{"name": "echo", "args": []string{"hosted", "round trip"}})
	if echo.ExitCode != 0 || echo.Stdout != "hosted round trip\n" {
		t.Fatalf("echo = %+v", echo)
	}
	mcpHostedScratchEmpty(t, h)

	// The same identity never executes a second time.
	replay := mcpHostedCallTool(t, ctx, cs, "run_tool", map[string]any{"name": "echo", "args": []string{"again"}})
	if replay.State != "replayed" || replay.Stdout != "" {
		t.Fatalf("replayed call = %+v, want state replayed", replay)
	}

	// The call sees its scratch home and identity, never the runner's, and a
	// client cannot override them.
	second := mcpHostedFixtureIdentity("call-2")
	transport.set(mcpHostedToken(t, second, mcpHostedFixtureKey))
	// The script tool takes the same child-process path as run_tool.
	// The shell masks its own home in what it prints, so the script compares
	// the scratch paths itself and reports the verdicts.
	script := `echo "BASHY_HOSTED_SCRATCH=$BASHY_HOSTED_SCRATCH"
[ "$HOME" = "$BASHY_HOSTED_SCRATCH/home" ] && echo HOME=scratch
[ "$BASHY_HOME" = "$BASHY_HOSTED_SCRATCH/home/.bashy" ] && echo BASHY_HOME=scratch
[ "$TMPDIR" = "$BASHY_HOSTED_SCRATCH/tmp" ] && echo TMPDIR=scratch
[ "$(pwd -P)" = "$(cd "$BASHY_HOSTED_SCRATCH/work" && pwd -P)" ] && echo PWD=scratch
echo "BASHY_PRINCIPAL=$BASHY_PRINCIPAL"
echo "BASHY_HOSTED_TASK_ID=$BASHY_HOSTED_TASK_ID"
echo "BASHY_SECRETS_TOKEN=$BASHY_SECRETS_TOKEN"
echo "` + mcpHostedKeyEnv + `=${` + mcpHostedKeyEnv + `-unset}"
`
	env := mcpHostedCallTool(t, ctx, cs, "bashy", map[string]any{"script": script})
	if env.ExitCode != 0 {
		t.Fatalf("script = %+v", env)
	}
	got := map[string]string{}
	for _, line := range strings.Split(env.Stdout, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			got[key] = value
		}
	}
	root := got["BASHY_HOSTED_SCRATCH"]
	if root == "" || filepath.Dir(root) != h.scratch {
		t.Fatalf("scratch %q is not under the scratch root %q", root, h.scratch)
	}
	for key, want := range map[string]string{
		"HOME": "scratch", "BASHY_HOME": "scratch", "TMPDIR": "scratch", "PWD": "scratch", mcpHostedKeyEnv: "unset",
		"BASHY_PRINCIPAL": second.Caller, "BASHY_HOSTED_TASK_ID": second.TaskID, "BASHY_SECRETS_TOKEN": "",
	} {
		if got[key] != want {
			t.Errorf("%s = %q, want %q", key, got[key], want)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("scratch %s still exists after the call: %v", root, err)
	}
	mcpHostedScratchEmpty(t, h)

	// A file the call writes lands in its scratch work directory and goes
	// with it; a client cannot move the call's home through env.
	transport.set(mcpHostedToken(t, mcpHostedFixtureIdentity("call-3"), mcpHostedFixtureKey))
	if out := mcpHostedCallTool(t, ctx, cs, "run_tool", map[string]any{"name": "touch", "args": []string{"left-behind"},
		"env": map[string]string{"TMPDIR": "/elsewhere"}}); out.ExitCode != 0 {
		t.Fatalf("touch = %+v", out)
	}
	mcpHostedScratchEmpty(t, h)
	transport.set(mcpHostedToken(t, mcpHostedFixtureIdentity("call-4"), mcpHostedFixtureKey))
	if out := mcpHostedCallTool(t, ctx, cs, "run_tool", map[string]any{"name": "echo", "dir": "../../escape"}); out.ExitCode != 2 {
		t.Fatalf("escaping dir = %+v, want exit 2", out)
	}

	// Every call's audit line names the caller as the principal.
	data, err := os.ReadFile(auditLog)
	if err != nil {
		t.Fatal(err)
	}
	decisions := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record struct {
			Actor struct {
				Human   string `json:"human"`
				Session string `json:"session"`
			} `json:"actor"`
			Action   string `json:"action"`
			Binary   string `json:"binary"`
			Decision string `json:"decision"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("audit line %q: %v", line, err)
		}
		if record.Actor.Human != first.Caller || record.Actor.Session != first.TaskID {
			t.Fatalf("audit line %q: principal %q session %q", line, record.Actor.Human, record.Actor.Session)
		}
		decisions[record.Binary] = record.Decision
	}
	if decisions["rm"] != "deny" || decisions["echo"] != "allow" {
		t.Fatalf("audit decisions = %v", decisions)
	}
}

func TestMCPHostedVaultProjection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the child process of this test is the unix test binary")
	}
	const prefix = "/api/v1/secrets"
	const googlePath = "google/google-test@example.test/credentials"
	vault := map[string]string{
		googlePath: "{}",
		"google/google-test@example.test/access-token": "gcloud-fixture",
		"github/test/token":                            "github-fixture",
		"aws/test/access-key-id":                       "access-fixture",
		"aws/test/secret-access-key":                   "secret-fixture",
		"aws/test/session-token":                       "session-fixture",
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if r.Method != http.MethodGet || (token != "Bearer fixture-vault" && token != "Bearer fixture-missing") {
			t.Error("unexpected request to the fake vault")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == prefix {
			var names []map[string]string
			for name := range vault {
				names = append(names, map[string]string{"name": name})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": names})
			return
		}
		name := strings.TrimPrefix(r.URL.Path, prefix+"/")
		value, ok := vault[name]
		if !ok || (token == "Bearer fixture-missing" && name == googlePath) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"name": name, "value": value})
	}))
	defer fake.Close()
	h, _ := mcpHostedFixture(t, time.Minute, defaultMCPMaxOutput)
	t.Setenv("BASHY_CLOUDBOX_URL", fake.URL)
	t.Setenv("BASHY_SECRETS_TOKEN", "runner-fixture")
	t.Setenv("GH_TOKEN", "runner-github-fixture")
	writeRecord(t, fleet.Command{Name: "trove", Synopsis: "vault-aware fixture",
		Script: `[ "$BASHY_SECRETS_TOKEN" = fixture-vault ] && echo vault-reader`, Effects: []string{"read"}})
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	transport := &mcpHostedTransport{}
	id := mcpHostedFixtureIdentity("vault-projection")
	id.SecretsToken = "fixture-vault"
	transport.set(mcpHostedToken(t, id, mcpHostedFixtureKey))
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx,
		&mcpsdk.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: transport}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	const script = `set -e
[ "$GH_TOKEN" = github-fixture ]
[ "$CLOUDSDK_AUTH_ACCESS_TOKEN" = gcloud-fixture ]
[ "$AWS_ACCESS_KEY_ID" = access-fixture ]
[ "$AWS_SECRET_ACCESS_KEY" = secret-fixture ]
[ "$AWS_SESSION_TOKEN" = session-fixture ]
[ -z "$BASHY_SECRETS_TOKEN" ]
[ "$BASHY_HOSTED_GOOGLE_NAMES" = "google/google-test@example.test/credentials" ]
[ "$GWS_CONFIG_ROOT" = "$HOME/.config/bashy/google" ]
file="$GWS_CONFIG_ROOT/google-test@example.test/credentials.json"
[ "$(stat -c %a "$file")" = 600 ]
[ "$(cat "$file")" = '{}' ]
echo projected
`
	if got := mcpHostedCallTool(t, ctx, cs, "bashy", map[string]any{"script": script}); got.ExitCode != 0 || got.Stdout != "projected\n" {
		t.Fatalf("projection call = %+v", got)
	}
	mcpHostedScratchEmpty(t, h)

	id.IdempotencyKey = "vault-reader"
	transport.set(mcpHostedToken(t, id, mcpHostedFixtureKey))
	if got := mcpHostedCallTool(t, ctx, cs, "trove", map[string]any{}); got.ExitCode != 0 || got.Stdout != "vault-reader\n" {
		t.Fatalf("registered trove call = %+v", got)
	}
	mcpHostedScratchEmpty(t, h)

	id.IdempotencyKey, id.SecretsToken = "vault-missing", "fixture-missing"
	transport.set(mcpHostedToken(t, id, mcpHostedFixtureKey))
	if got := mcpHostedCallTool(t, ctx, cs, "bashy", map[string]any{"script": "echo should-not-run"}); got.ExitCode != 1 ||
		got.Stdout != "" || got.Stderr != "no such secret: "+googlePath+"\n" {
		t.Fatalf("missing secret call = %+v", got)
	}
	mcpHostedScratchEmpty(t, h)

	id.IdempotencyKey, id.SecretsToken = "vault-none", ""
	transport.set(mcpHostedToken(t, id, mcpHostedFixtureKey))
	if got := mcpHostedCallTool(t, ctx, cs, "bashy", map[string]any{"script": `set -e
[ -z "${GH_TOKEN:-}${AWS_ACCESS_KEY_ID:-}${CLOUDSDK_AUTH_ACCESS_TOKEN:-}${BASHY_SECRETS_TOKEN:-}" ]
[ ! -e "$GWS_CONFIG_ROOT/google-test@example.test/credentials.json" ]
echo clean
`}); got.ExitCode != 0 || got.Stdout != "clean\n" {
		t.Fatalf("call without a token = %+v", got)
	}
	mcpHostedScratchEmpty(t, h)
}

func TestMCPHostedLimits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the child process of this test is the unix test binary")
	}
	h, _ := mcpHostedFixture(t, 2*time.Second, 8)
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	transport := &mcpHostedTransport{}
	transport.set(mcpHostedToken(t, mcpHostedFixtureIdentity("cap"), mcpHostedFixtureKey))
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx,
		&mcpsdk.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: transport}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	capped := mcpHostedCallTool(t, ctx, cs, "run_tool", map[string]any{"name": "echo", "args": []string{"0123456789abcdef"}})
	if capped.Stdout != "01234567" || !strings.Contains(capped.Stderr, "stdout truncated at 8 bytes (9 dropped)") {
		t.Fatalf("capped call = %+v", capped)
	}
	transport.set(mcpHostedToken(t, mcpHostedFixtureIdentity("slow"), mcpHostedFixtureKey))
	start := time.Now()
	slow := mcpHostedCallTool(t, ctx, cs, "run_tool", map[string]any{"name": "sleep", "args": []string{"60"}})
	if slow.ExitCode != mcpHostedTimeoutExit || time.Since(start) > 30*time.Second {
		t.Fatalf("slow call = %+v after %s, want exit %d at the deadline", slow, time.Since(start), mcpHostedTimeoutExit)
	}
	mcpHostedScratchEmpty(t, h)
}

func TestMCPHostedFlags(t *testing.T) {
	for _, args := range [][]string{
		{"serve", "--hosted"},
		{"serve", "--transport", "http", "--hosted", "--outpost"},
		{"serve", "--transport", "http", "--hosted", "--tools", "site", "--site-dir", "."},
		{"serve", "--transport", "http", "--timeout", "5s"},
		{"serve", "--transport", "http", "--hosted", "--timeout", "soon"},
		{"serve", "--transport", "http", "--hosted", "--listen", "192.0.2.1:0"},
	} {
		if code := dispatchMCP(args); code != 2 {
			t.Errorf("bashy mcp %s = %d, want 2", strings.Join(args, " "), code)
		}
	}
}
