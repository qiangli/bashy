package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/yoke/pkg/accounts"
	"github.com/qiangli/yoke/pkg/fleet/cloudboxfake"
)

type mcpConnectFixture struct {
	fake       *cloudboxfake.Fake
	satID      string
	satURL     string
	appURL     string
	shareID    string
	ownerTok   string
	memberTok  string
	outsider   string
	mu         sync.Mutex
	now        time.Time
	seenBearer []string
}

func newMCPConnectFixture(t *testing.T, shareWithMember, signedIn bool) *mcpConnectFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("BASHY_HOME", filepath.Join(home, "bashy"))
	t.Setenv("BASHY_FLEET_TOKEN", "")
	t.Setenv("BASHY_API_KEY", "")
	t.Setenv("BASHY_CLOUDBOX_URL", "")

	fx := &mcpConnectFixture{
		fake: cloudboxfake.New(t),
		now:  time.Date(2026, 11, 24, 23, 0, 0, 0, time.UTC),
	}
	fx.fake.SetNow(fx.nowTime)

	fx.ownerTok = fx.fake.AddOwner("owner@example.com")
	fx.memberTok = fx.fake.AddOwner("member@example.com")
	fx.outsider = fx.fake.AddOwner("outsider@example.com")
	fx.satID = fx.fake.AddSatellite(t, "owner@example.com", "zoo")
	fx.satURL = fx.fake.SatelliteURL(fx.satID)
	fx.appURL = fx.satURL + "/matrix/h/studio/app/site/mcp"

	send := func(method, path, tok string, body any) map[string]any {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req, err := http.NewRequest(method, fx.fake.URL()+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			t.Fatalf("%s %s failed with HTTP %d", method, path, resp.StatusCode)
		}
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}

	pair := send("POST", "/api/v1/hosts/invites", fx.ownerTok, nil)
	send("POST", "/api/v1/hosts/pair", "", map[string]string{"code": pair["code"].(string), "host": "studio"})
	added := send("POST", "/api/v1/members", fx.ownerTok, map[string]string{"email": "member@example.com", "circle": "contractor"})
	invID := added["invitation"].(map[string]any)["id"].(string)
	send("POST", "/api/v1/member/invitations/"+invID+"/accept", fx.memberTok, map[string]string{"circle": "team"})
	send("PUT", "/api/v1/satellites/"+fx.satID+"/admit", fx.ownerTok, map[string]any{"rules": []cloudboxfake.AdmitRule{{Circles: []string{"contractor"}}}})
	if signedIn {
		fx.fake.RecordSatelliteLogin(fx.satID, "member@example.com")
	}
	if shareWithMember {
		shr := send("POST", "/api/v1/hosts/studio/shares", fx.ownerTok, map[string]any{"app": "site", "circles": []string{"contractor"}})
		fx.shareID = shr["share"].(map[string]any)["id"].(string)
	}

	// Build a real httptest Streamable HTTP MCP server exposing site-like tools.
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "site-mcp-fixture", Version: "1.0.0"}, nil)
	type echoIn struct {
		Page string `json:"page"`
	}
	type echoOut struct {
		Page   string `json:"page"`
		Caller string `json:"caller"`
	}
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "read_page",
		Description: "Read a page from the owner's site.",
	}, func(_ context.Context, req *mcpsdk.CallToolRequest, in echoIn) (*mcpsdk.CallToolResult, echoOut, error) {
		c := req.Extra.Header.Get("X-Periscope-User")
		return nil, echoOut{Page: in.Page, Caller: c}, nil
	})
	type listIn struct{}
	type listOut struct {
		Pages []string `json:"pages"`
	}
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "list_pages",
		Description: "List site pages.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, _ listIn) (*mcpsdk.CallToolResult, listOut, error) {
		return nil, listOut{Pages: []string{"/", "/about"}}, nil
	})

	streamHandler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return srv }, &mcpsdk.StreamableHTTPOptions{Stateless: true})
	backend := httptest.NewServer(streamHandler)
	t.Cleanup(backend.Close)
	upstream, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	fx.fake.MountApp(fx.satID, "studio", "site", httputil.NewSingleHostReverseProxy(upstream))
	return fx
}

func (fx *mcpConnectFixture) advance(d time.Duration) {
	fx.mu.Lock()
	fx.now = fx.now.Add(d)
	fx.mu.Unlock()
}

func (fx *mcpConnectFixture) nowTime() time.Time {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	return fx.now
}

func (fx *mcpConnectFixture) recordingHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasPrefix(r.URL.Path, "/matrix/h/") {
				auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				fx.mu.Lock()
				fx.seenBearer = append(fx.seenBearer, auth)
				fx.mu.Unlock()
			}
			return http.DefaultTransport.RoundTrip(r)
		}),
	}
}

func (fx *mcpConnectFixture) bearers() []string {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	return append([]string(nil), fx.seenBearer...)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func seedCloudboxAccount(t *testing.T, alias, endpoint, envVar, tokenVal string) {
	t.Helper()
	t.Setenv(envVar, tokenVal)
	st := accounts.Open(accounts.DefaultPath())
	if err := st.Observe("cloudbox", []accounts.Observation{{
		SourceKey: alias,
		Account: accounts.Account{
			Alias:             alias,
			Category:          accounts.Cloudbox,
			Provider:          "cloudbox",
			ProviderAccountID: alias,
			Connections: []accounts.Connection{{
				ID:            "primary",
				Endpoint:      endpoint,
				CredentialRef: "env:" + envVar,
			}},
		},
	}}); err != nil {
		t.Fatalf("seed account %s: %v", alias, err)
	}
}

func assertNoTokensInOutput(t *testing.T, text string, extra ...string) {
	t.Helper()
	for _, secret := range extra {
		if secret != "" && strings.Contains(text, secret) {
			t.Fatal("credential leaked in output")
		}
	}
	for _, prefix := range []string{"tok_", "xtok_"} {
		if strings.Contains(text, prefix) {
			t.Fatal("fake credential leaked in output")
		}
	}
}

func TestMCPConnectStdioPassThroughAndRefreshPastTTL(t *testing.T) {
	fx := newMCPConnectFixture(t, true, true)
	seedCloudboxAccount(t, "contractor-acct", fx.fake.URL(), "TEST_CONTRACTOR_TOKEN", fx.memberTok)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Use the same newline-delimited JSON as stdio and inspect the actual wire output.
	clientRead, serverWrite := io.Pipe()
	serverRead, clientWrite := io.Pipe()
	var wire bytes.Buffer
	serverSide := &mcpsdk.IOTransport{Reader: serverRead, Writer: serverWrite}
	clientSide := &mcpsdk.IOTransport{Reader: struct {
		io.Reader
		io.Closer
	}{io.TeeReader(clientRead, &wire), clientRead}, Writer: clientWrite}
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- runMCPConnect(ctx, []string{fx.appURL, "--account", "contractor-acct"}, mcpConnectDeps{
			stdio:      serverSide,
			stdout:     &stdout,
			stderr:     &stderr,
			httpClient: fx.recordingHTTPClient(),
			now:        fx.nowTime,
		})
	}()

	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "claude-code-test", Version: "1"}, nil).Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatalf("local stdio client connect: %v", err)
	}

	// 1. tools/list passes through.
	toolsRes, err := cs.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var toolNames []string
	for _, tool := range toolsRes.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	slices.Sort(toolNames)
	if !slices.Equal(toolNames, []string{"list_pages", "read_page"}) {
		t.Fatalf("tools/list = %v, want [list_pages read_page]", toolNames)
	}

	// 2. tools/call passes through with the member's X-Periscope-User identity.
	callRes, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "read_page",
		Arguments: map[string]any{"page": "/about"},
	})
	if err != nil || callRes.IsError {
		t.Fatalf("CallTool before TTL: err=%v res=%+v", err, callRes)
	}
	raw, _ := json.Marshal(callRes.StructuredContent)
	if !strings.Contains(string(raw), `"/about"`) || !strings.Contains(string(raw), `"member@example.com"`) {
		t.Fatalf("CallTool structured content = %s", raw)
	}

	firstBearers := fx.bearers()
	if len(firstBearers) == 0 {
		t.Fatal("expected HTTP requests to carry Bearer token")
	}
	tok1 := firstBearers[0]
	if !strings.HasPrefix(tok1, "xtok_") || tok1 == fx.memberTok {
		t.Fatal("expected exchanged satellite bearer")
	}
	for _, b := range firstBearers {
		if b != tok1 {
			t.Fatal("bearer changed within TTL")
		}
	}

	// 3. Advance clock past 10m TTL: tok1 is now expired on cloudboxfake;
	// next tools/list and tools/call over the SAME stdio session must refresh!
	fx.advance(11 * time.Minute)

	if _, err := cs.ListTools(ctx, &mcpsdk.ListToolsParams{}); err != nil {
		t.Fatalf("ListTools after TTL expiry: %v", err)
	}
	callRes2, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "read_page",
		Arguments: map[string]any{"page": "/pricing"},
	})
	if err != nil || callRes2.IsError {
		t.Fatalf("CallTool after TTL expiry: err=%v res=%+v", err, callRes2)
	}
	raw2, _ := json.Marshal(callRes2.StructuredContent)
	if !strings.Contains(string(raw2), `"/pricing"`) {
		t.Fatalf("CallTool after refresh = %s", raw2)
	}

	allBearers := fx.bearers()
	tok2 := allBearers[len(allBearers)-1]
	if !strings.HasPrefix(tok2, "xtok_") || tok2 == tok1 || tok2 == fx.memberTok {
		t.Fatal("expected fresh satellite bearer after expiry")
	}

	// 4. Revoking the owner's share mid-session causes the next call to fail
	// naming 'not shared with you'.
	delReq, _ := http.NewRequest(http.MethodDelete, fx.fake.URL()+"/api/v1/hosts/studio/shares/"+fx.shareID, nil)
	delReq.Header.Set("Authorization", "Bearer "+fx.ownerTok)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil || delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete share: %v %v", err, delResp)
	}
	delResp.Body.Close()

	_, err = cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "read_page",
		Arguments: map[string]any{"page": "/about"},
	})
	if err == nil || !strings.Contains(err.Error(), "not shared with you") {
		t.Fatalf("CallTool after share revoked = %v, want 'not shared with you'", err)
	}

	// Closing client cleanly terminates runMCPConnect with exit 0 and zero leaked tokens.
	_ = cs.Close()
	if code := <-done; code != 0 {
		t.Fatalf("runMCPConnect exit = %d, want 0 (stderr=%s)", code, stderr.String())
	}
	assertNoTokensInOutput(t, wire.String()+stdout.String()+stderr.String(), fx.memberTok, fx.ownerTok, tok1, tok2)
}

func TestMCPConnectJSONEnvelopeAndZeroTokenLeakage(t *testing.T) {
	fx := newMCPConnectFixture(t, true, true)
	seedCloudboxAccount(t, "c-acct", fx.fake.URL(), "TEST_C_TOKEN", fx.memberTok)

	var stdout, stderr bytes.Buffer
	code := runMCPConnect(context.Background(), []string{fx.appURL, "--account", "c-acct", "--json"}, mcpConnectDeps{
		stdout:     &stdout,
		stderr:     &stderr,
		httpClient: fx.recordingHTTPClient(),
		now:        fx.nowTime,
	})
	if code != 0 {
		t.Fatalf("mcp connect --json exited %d: stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	assertNoTokensInOutput(t, stdout.String()+stderr.String(), fx.memberTok, fx.ownerTok)
	for _, b := range fx.bearers() {
		assertNoTokensInOutput(t, stdout.String()+stderr.String(), b)
	}

	var env mcpConnectEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid --json envelope: %v\n%s", err, stdout.String())
	}
	if env.SchemaVersion != mcpConnectSchemaVersion || !env.OK {
		t.Fatalf("envelope header = %+v", env)
	}
	if !slices.Equal(env.Tools, []string{"list_pages", "read_page"}) {
		t.Fatalf("envelope tools = %v", env.Tools)
	}
}

func TestMCPConnectErrorsNameCauseAndScrubTokens(t *testing.T) {
	fx := newMCPConnectFixture(t, false, true)
	beforeSignIn := newMCPConnectFixture(t, true, false)
	for _, tc := range []struct {
		name, target, token, code, want string
	}{
		{"no account token", fx.appURL, "", "no_account_token", "no account token"},
		{"not admitted", fx.appURL, fx.outsider, "exchange_refused", "admitted"},
		{"missing delegation scope", fx.appURL, fx.fake.AddToken("member@example.com", cloudboxfake.ScopeHostProxy), "exchange_refused", "satellites:delegate"},
		{"missing requested scope", fx.appURL, fx.fake.AddToken("member@example.com", cloudboxfake.ScopeSatellitesDelegate), "exchange_refused", "host:proxy"},
		{"invalid token", fx.appURL, "sentinel-invalid-account", "exchange_refused", "invalid or expired"},
		{"never signed in", beforeSignIn.appURL, beforeSignIn.memberTok, "exchange_refused", "sign in to this satellite in the browser"},
		{"app not shared", fx.appURL, fx.memberTok, "not_shared", "not shared with you"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASHY_FLEET_TOKEN", tc.token)
			var stdout, stderr bytes.Buffer
			exit := runMCPConnect(context.Background(), []string{tc.target, "--json"}, mcpConnectDeps{
				stdout: &stdout, stderr: &stderr, now: fx.nowTime,
			})
			assertNoTokensInOutput(t, stdout.String()+stderr.String(), tc.token)
			var env mcpConnectEnvelope
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			if exit == 0 || env.OK || env.SchemaVersion != mcpConnectSchemaVersion || env.Code != tc.code {
				t.Fatalf("exit %d, envelope %+v", exit, env)
			}
			if !strings.Contains(stderr.String(), tc.want) || !strings.Contains(env.Error, tc.want) {
				t.Fatalf("expected %q in stderr and envelope: %q / %q", tc.want, stderr.String(), env.Error)
			}
		})
	}
}

func TestDispatchMCPConnectFrontDoorValidation(t *testing.T) {
	if code := dispatchMCP([]string{"connect", "--help"}); code != 2 {
		t.Fatalf("connect --help = %d, want 2", code)
	}
	const secret = "cli-secret-must-never-appear-123"
	stderr := captureMCPStderr(t, func() {
		if code := dispatchMCP([]string{"connect", "https://user:" + secret + "@example.com/matrix/h/studio/app/site/mcp"}); code != 2 {
			t.Fatalf("connect with URL credentials = %d, want 2", code)
		}
	})
	if strings.Contains(stderr, secret) {
		t.Fatal("credential leaked in stderr")
	}
	if !strings.Contains(stderr, "invalid satellite app URL") {
		t.Fatalf("expected invalid URL error, got %q", stderr)
	}
}
