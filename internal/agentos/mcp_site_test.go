package agentos

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	yokemcp "github.com/qiangli/yoke/mcp"
)

const siteTestKey = "site-test-key"

// fakeSite serves the template's wire shape: /llms.txt, JSON and Markdown
// twins, and the negotiated HTML page.
func fakeSite(t *testing.T) *httptest.Server {
	t.Helper()
	md := "We are three people.\n"
	page := map[string]any{"path": "/about", "title": "About", "summary": "Who we are", "revision": "abc123def456",
		"sections": []map[string]string{{"id": "team", "heading": "Team", "markdown": md, "sha256": siteSectionSHA256("team", "Team", md)}}}
	home := map[string]any{"path": "/", "title": "Home", "revision": "000111222333", "sections": []any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/llms.txt":
			w.Write([]byte("# Example\n\n## Pages\n\n" +
				"- [Home](https://example.com/): markdown https://example.com/index.md · json https://example.com/index.json · revision 000111222333\n" +
				"- [About](https://example.com/about): markdown https://example.com/about.md · json https://example.com/about.json · revision abc123def456\n"))
		case "/about.json":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(page)
		case "/index.json":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(home)
		case "/about.md":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.Write([]byte("# About\n\n## Team {#team}\n\n" + md))
		case "/about":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<section id="team"></section>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writeSiteOutpostConfig is the host-private outpost registration of app
// "site": its signing key, login posture and loopback upstream.
func writeSiteOutpostConfig(t *testing.T, upstream string, requireLogin bool) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(upstream, "http://"))
	n, _ := strconv.Atoi(port)
	data, _ := json.Marshal(map[string]any{"apps": []map[string]any{{"name": "site", "scheme": "http", "host": host, "port": n,
		"require_login": requireLogin, "sso_secret": siteTestKey}}})
	if err := os.MkdirAll(filepath.Join(dir, "matrix"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "matrix", "agent.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// stamp signs the headers exactly as outpost's app proxy does (apps.go).
func stamp(h http.Header, key, kind, user, visitor string) {
	prefix, ts := "/h/host/app/site", strconv.FormatInt(time.Now().Unix(), 10)
	h.Set("X-Forwarded-Prefix", prefix)
	if user != "" {
		h.Set("Remote-User", user)
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(user + "\n" + "" + "\n" + prefix + "\n" + ts))
	h.Set("X-Outpost-Identity-Sig", hex.EncodeToString(mac.Sum(nil)))
	h.Set("X-Outpost-Identity-Ts", ts)
	if kind != "" {
		h.Set("X-Outpost-Relationship", kind)
		if visitor != "" {
			h.Set("X-Outpost-Visitor", visitor)
		}
		rmac := hmac.New(sha256.New, []byte(key))
		rmac.Write([]byte("v2\n" + kind + "\n\n\n" + visitor + "\n" + user + "\n" + prefix + "\n" + ts))
		h.Set("X-Outpost-Relationship-Sig", hex.EncodeToString(rmac.Sum(nil)))
	}
}

type headerTransport struct{ set func(http.Header) }

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if h.set != nil {
		h.set(r.Header)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// serveSiteHTTP serves the site profile over the real loopback Streamable
// HTTP transport and returns a connect function for a client whose every
// request carries the given stamp.
func serveSiteHTTP(t *testing.T, s *siteMCP) func(func(http.Header)) *mcpsdk.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	srv := newSiteMCPServer(s, "test", func(yokemcp.Record) {})
	addr, shutdown, err := yokemcp.ServeHTTPServerWithAuth(ctx, srv, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = shutdown() })
	return func(set func(http.Header)) *mcpsdk.ClientSession {
		transport := &mcpsdk.StreamableClientTransport{Endpoint: "http://" + addr + "/mcp",
			HTTPClient: &http.Client{Transport: headerTransport{set: set}}}
		cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "t", Version: "1"}, nil).Connect(ctx, transport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cs.Close() })
		return cs
	}
}

func callSite(t *testing.T, cs *mcpsdk.ClientSession, name string, args map[string]any) (map[string]any, string) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		return nil, res.Content[0].(*mcpsdk.TextContent).Text
	}
	data, _ := json.Marshal(res.StructuredContent)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out, ""
}

func stubSiteShape(t *testing.T) *[]shapeCLIRequest {
	t.Helper()
	var calls []shapeCLIRequest
	oldExec, oldActor := siteShapeExecute, shapeAuthoredActor
	siteShapeExecute = func(_ context.Context, r shapeCLIRequest) (any, error) {
		calls = append(calls, r)
		return map[string]any{"id": "c1", "status": "ready"}, nil
	}
	shapeAuthoredActor = func(string) (string, error) { return "site-owner-agent", nil }
	t.Cleanup(func() { siteShapeExecute, shapeAuthoredActor = oldExec, oldActor })
	return &calls
}

func TestSiteMCPToolsAreExactlyTheContract(t *testing.T) {
	defs, err := siteToolDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range defs {
		names = append(names, d.Name)
	}
	want := slices.Clone(siteMCPTools)
	slices.Sort(names)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("site tools = %v, want exactly %v (no shell, run_tool or registry tools)", names, want)
	}
}

func TestSiteMCPReadToolsOverHTTP(t *testing.T) {
	site := fakeSite(t)
	writeSiteOutpostConfig(t, site.URL, false)
	connect := serveSiteHTTP(t, newSiteMCP(context.Background(), "site", "", true))
	cs := connect(nil) // anonymous, public site

	out, refusal := callSite(t, cs, "list_pages", nil)
	if refusal != "" {
		t.Fatal(refusal)
	}
	pages := out["pages"].([]any)
	about := pages[1].(map[string]any)
	if len(pages) != 2 || about["path"] != "/about" || about["summary"] != "Who we are" || about["revision"] != "abc123def456" ||
		about["twins"].(map[string]any)["json"] != "https://example.com/about.json" {
		t.Fatalf("list_pages = %v", out)
	}
	for format, want := range map[string]string{"": "## Team {#team}", "json": `"sha256"`, "html": `<section id="team">`} {
		out, refusal = callSite(t, cs, "read_page", map[string]any{"path_or_url": "https://example.com/about", "format": format})
		if refusal != "" || out["revision"] != "abc123def456" || !strings.Contains(out["content"].(string), want) {
			t.Fatalf("read_page %q = %v %s", format, out, refusal)
		}
	}
	out, refusal = callSite(t, cs, "cite", map[string]any{"ref": "/about#team"})
	if refusal != "" || out["url"] != "https://example.com/about#team" || out["section_id"] != "team" ||
		out["revision"] != "abc123def456" || out["quote"] != "We are three people.\n" ||
		out["section_sha256"] != siteSectionSHA256("team", "Team", "We are three people.\n") {
		t.Fatalf("cite = %v %s", out, refusal)
	}
	if _, refusal = callSite(t, cs, "cite", map[string]any{"ref": "/about#nope"}); !strings.Contains(refusal, `no section "nope"`) {
		t.Fatalf("missing section: %s", refusal)
	}
}

func TestSiteMCPTiersOverHTTP(t *testing.T) {
	site := fakeSite(t)
	calls := stubSiteShape(t)
	for _, tc := range []struct {
		name, kind, user, visitor, key string
		requireLogin                   bool
		readOK, shapeOK                bool
		principal, tier                string
	}{
		{name: "owner", kind: "owner", user: "owner@example.com", key: siteTestKey, readOK: true, shapeOK: true},
		{name: "member", kind: "member", user: "friend@example.com", key: siteTestKey, readOK: true, principal: "friend@example.com", tier: "member"},
		{name: "signed-in visitor", kind: "signed-in", visitor: "v123", key: siteTestKey, readOK: true, principal: "visitor:v123", tier: "signed-in"},
		{name: "anonymous public", readOK: true, principal: "anonymous", tier: "anonymous"},
		{name: "anonymous login-required", requireLogin: true, principal: "anonymous", tier: "anonymous"},
		{name: "signed-in login-required", kind: "signed-in", visitor: "v123", key: siteTestKey, requireLogin: true, principal: "visitor:v123", tier: "signed-in"},
		{name: "member login-required", kind: "member", user: "friend@example.com", key: siteTestKey, requireLogin: true, readOK: true, principal: "friend@example.com", tier: "member"},
		{name: "forged owner stamp", kind: "owner", user: "owner@example.com", key: "wrong-key", readOK: true, principal: "anonymous", tier: "anonymous"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeSiteOutpostConfig(t, site.URL, tc.requireLogin)
			connect := serveSiteHTTP(t, newSiteMCP(context.Background(), "site", "", true))
			cs := connect(func(h http.Header) {
				if tc.key != "" {
					stamp(h, tc.key, tc.kind, tc.user, tc.visitor)
				}
			})
			_, refusal := callSite(t, cs, "read_page", map[string]any{"path_or_url": "/about"})
			if tc.readOK != (refusal == "") {
				t.Fatalf("read_page refusal = %q, want allowed=%v", refusal, tc.readOK)
			}
			if !tc.readOK && refusal != "refused: caller "+tc.principal+" (tier "+tc.tier+") is not authorized for read_page; member or owner tier required (the site requires login)" {
				t.Fatalf("read refusal = %q", refusal)
			}
			before := len(*calls)
			out, refusal := callSite(t, cs, "shape_wish", map[string]any{"request": "add a team photo", "element": "section=team revision=abc123def456"})
			if tc.shapeOK {
				if refusal != "" || out["principal"] != "owner@example.com" || len(*calls) != before+1 {
					t.Fatalf("owner shape_wish = %v %s", out, refusal)
				}
				r := (*calls)[len(*calls)-1]
				if r.Verb != "wish" || r.App != "site" || r.User != "owner@example.com" || r.Actor != "site-owner-agent" || r.Element != "section=team revision=abc123def456" {
					t.Fatalf("shape request = %+v", r)
				}
				for _, tool := range []string{"shape_verify", "shape_preview", "shape_approve"} {
					if _, refusal := callSite(t, cs, tool, map[string]any{"id": "c1"}); refusal != "" {
						t.Fatalf("%s: %s", tool, refusal)
					}
				}
				if _, refusal := callSite(t, cs, "shape_undo", nil); refusal != "" {
					t.Fatal(refusal)
				}
				return
			}
			want := "refused: caller " + tc.principal + " (tier " + tc.tier + ") is not authorized for shape_wish; owner tier required"
			if refusal != want || len(*calls) != before {
				t.Fatalf("shape_wish refusal = %q, want %q (shape calls %d→%d)", refusal, want, before, len(*calls))
			}
			for _, tool := range []string{"shape_verify", "shape_preview", "shape_approve", "shape_undo"} {
				args := map[string]any{"id": "c1"}
				if tool == "shape_undo" {
					args = nil
				}
				if _, refusal := callSite(t, cs, tool, args); !strings.HasPrefix(refusal, "refused: caller "+tc.principal+" (tier "+tc.tier+") is not authorized for "+tool) {
					t.Fatalf("%s refusal = %q", tool, refusal)
				}
			}
		})
	}
}

func TestSiteMCPLocalOwnerWithoutOutpost(t *testing.T) {
	site := fakeSite(t)
	calls := stubSiteShape(t)
	t.Setenv("BASHY_PRINCIPAL", "local-owner")
	s := newSiteMCP(context.Background(), "site", "", false)
	s.once.Do(func() { s.base = site.URL })
	cs := serveSiteHTTP(t, s)(nil)
	if out, refusal := callSite(t, cs, "shape_undo", nil); refusal != "" || out["principal"] != "local-owner" || (*calls)[0].Verb != "undo" {
		t.Fatalf("local owner shape_undo = %v %s", out, refusal)
	}
}

func TestSiteMCPAuditRecordsRefusalByName(t *testing.T) {
	site := fakeSite(t)
	writeSiteOutpostConfig(t, site.URL, false)
	var records []yokemcp.Record
	ctx := context.Background()
	srv := newSiteMCPServer(newSiteMCP(ctx, "site", "", true), "test", func(r yokemcp.Record) { records = append(records, r) })
	st, ct := mcpsdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "t", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	callSite(t, cs, "shape_undo", nil)
	if len(records) != 1 || records[0].Allowed || records[0].ExitCode != 126 ||
		records[0].Denial != "refused: caller anonymous (tier anonymous) is not authorized for shape_undo; owner tier required" {
		t.Fatalf("audit = %+v", records)
	}
}

// The real site template renders the pages; the profile must cite with the
// section hashes the template computes, through its own server.
func TestSiteMCPReadsRealTemplate(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the site template's server")
	}
	dir, err := filepath.Abs("../../examples/shaping/site/template")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newSiteMCP(ctx, "", dir, false)
	srv := newSiteMCPServer(s, "test", nil)
	st, ct := mcpsdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "t", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	out, refusal := callSite(t, cs, "list_pages", nil)
	if refusal != "" {
		t.Fatal(refusal)
	}
	var about map[string]any
	for _, p := range out["pages"].([]any) {
		if p.(map[string]any)["path"] == "/about" {
			about = p.(map[string]any)
		}
	}
	if about == nil {
		t.Fatalf("no /about in %v", out)
	}
	page, refusal := callSite(t, cs, "read_page", map[string]any{"path_or_url": "/about", "format": "json"})
	if refusal != "" {
		t.Fatal(refusal)
	}
	var doc sitePageJSON
	if err := json.Unmarshal([]byte(page["content"].(string)), &doc); err != nil || len(doc.Sections) == 0 {
		t.Fatalf("json twin: %v %v", err, page)
	}
	section := doc.Sections[0]
	cite, refusal := callSite(t, cs, "cite", map[string]any{"ref": "https://example.com/about#" + section.ID})
	if refusal != "" || cite["section_sha256"] != section.SHA256 || cite["revision"] != about["revision"] || cite["quote"] != section.Markdown {
		t.Fatalf("cite = %v %s, template section %+v", cite, refusal, section)
	}
}
