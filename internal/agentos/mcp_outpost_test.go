package agentos

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	yokemcp "github.com/qiangli/yoke/mcp"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"
)

const mcpFixtureKey = "unit-test-app-key"

func mcpSignedRequest(r *http.Request, key string) {
	r.Header.Set("Remote-User", "test-caller")
	r.Header.Set("X-Forwarded-Prefix", "/app/bashy-mcp")
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	r.Header.Set("X-Outpost-Identity-Ts", ts)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("test-caller\n\n/app/bashy-mcp\n" + ts))
	r.Header.Set("X-Outpost-Identity-Sig", hex.EncodeToString(mac.Sum(nil)))
}
func mcpFixtureConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	dir = filepath.Join(dir, "matrix")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agent.json")
	if err := os.WriteFile(path, []byte(`{"service_sso":{"bashy-mcp":"`+mcpFixtureKey+`"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestMCPOutpostAuthentication(t *testing.T) {
	path := mcpFixtureConfig(t)
	for _, tc := range []string{"valid", "unsigned", "wrong-key", "wrong-user", "wrong-app", "expired", "non-loopback", "alias-conflict", "missing-key"} {
		t.Run(tc, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://localhost/mcp", nil)
			r.RemoteAddr = "127.0.0.1:1234"
			mcpSignedRequest(r, mcpFixtureKey)
			switch tc {
			case "unsigned":
				r.Header.Del("X-Outpost-Identity-Sig")
			case "wrong-key":
				mcpSignedRequest(r, "other-test-key")
			case "wrong-user":
				r.Header.Set("Remote-User", "other-caller")
			case "wrong-app":
				r.Header.Set("X-Forwarded-Prefix", "/app/other")
			case "expired":
				r.Header.Set("X-Outpost-Identity-Ts", "1")
			case "non-loopback":
				r.RemoteAddr = "192.0.2.1:1234"
			case "alias-conflict":
				r.Header.Set("X-Periscope-User", "other-caller")
			case "missing-key":
				if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := mcpOutpostAuthenticate(r); got != (tc == "valid") {
				t.Fatalf("authentication = %v", got)
			}
		})
	}
}

type mcpSignedTransport struct{}

func (mcpSignedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	mcpSignedRequest(r, mcpFixtureKey)
	return http.DefaultTransport.RoundTrip(r)
}

func TestMCPOutpostToolsMatchCLI(t *testing.T) {
	ringDir(t)
	mcpFixtureConfig(t)
	var envelope mcpToolsEnvelope
	raw := captureStdout(t, func() {
		if dispatchMCPTools([]string{"--json"}) != 0 {
			t.Fatal("tools failed")
		}
	})
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatal(err)
	}
	opts, err := mcpOptions("", "default")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := yokemcp.BuildServer("bashy", "test", opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shells := registerMCPShells(ctx, srv, defaultMCPMaxOutput)
	defer func() { cancel(); <-shells.done }()
	addr, stop, err := yokemcp.ServeHTTPServerWithAuth(ctx, srv, "127.0.0.1:0", mcpOutpostAuthenticate)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	resp, err := http.Get("http://" + addr + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unsigned status %d", resp.StatusCode)
	}
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: "http://" + addr + "/mcp", HTTPClient: &http.Client{Transport: mcpSignedTransport{}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	var got []*mcpsdk.Tool
	params := &mcpsdk.ListToolsParams{}
	for {
		page, err := cs.ListTools(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, page.Tools...)
		if page.NextCursor == "" {
			break
		}
		params.Cursor = page.NextCursor
	}
	// Compare serialized definitions so interface-backed schemas have the same representation.
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(envelope.Definitions)
	var av, bv any
	json.Unmarshal(a, &av)
	json.Unmarshal(b, &bv)
	if !reflect.DeepEqual(av, bv) {
		t.Fatal("HTTP tools/list differs from CLI definitions")
	}
}

// An app published with `outpost apps add --trust-cloud-identity` signs with
// its own apps[].sso_secret; that key is accepted, another app's is not.
func TestMCPOutpostAuthenticationAppsKey(t *testing.T) {
	path := mcpFixtureConfig(t)
	cfg := `{"apps":[{"name":"bashy-mcp","sso_secret":"app-published-key"},{"name":"other","sso_secret":"other-app-key"}]}`
	if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{"app-published-key": true, "other-app-key": false, mcpFixtureKey: false} {
		r := httptest.NewRequest("POST", "http://localhost/mcp", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		mcpSignedRequest(r, key)
		if got := mcpOutpostAuthenticate(r); got != want {
			t.Fatalf("key %q: authentication = %v, want %v", key, got, want)
		}
	}
}
