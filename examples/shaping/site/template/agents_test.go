package site

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigDefaultsAndOverrides(t *testing.T) {
	s, err := LoadFS(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if s.Config.Module != "github.com/fixture/site" || s.Config.Source != "https://github.com/fixture/site" || s.Config.MCP != "https://fixture.example/mcp" || s.Config.Skill != "https://fixture.example/skill.md" || s.Config.Shape != "https://fixture.example/?shape=open" {
		t.Fatalf("defaults: %+v", s.Config)
	}
	if got := s.EditURL(s.ByPath["/about"]); got != "https://github.com/fixture/site/edit/main/content/about.md" {
		t.Fatalf("edit=%s", got)
	}
	if len(s.Config.AgentUserAgents) == 0 {
		t.Fatal("default agent UAs empty")
	}
}

func TestAgentChannels(t *testing.T) {
	loaded, srv, err := NewServerHandler(fixture, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	req := func(method, target, accept, ua string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, nil)
		if accept != "" {
			r.Header.Set("Accept", accept)
		}
		if ua != "" {
			r.Header.Set("User-Agent", ua)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, r)
		return rec
	}
	// agents.txt parse
	rec := req("GET", "/agents.txt", "", "")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("agents.txt %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	head, block, _ := strings.Cut(rec.Body.String(), "\n\n")
	for _, k := range []string{"site:", "title:", "module:", "revision:", "index:", "agents_md:", "markdown:", "json:", "skill:", "skills_index:", "mcp:", "mcp_card:", "source:", "edit:", "shape:", "cite:", "bashy:"} {
		if !strings.Contains(head, k) {
			t.Errorf("agents.txt missing %s", k)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(block), "## For agents {#agents}") {
		t.Errorf("agents.txt block: %q", block[:40])
	}
	// Bashy-Shaped + Link on page, HEAD, static, 404, 405
	for _, tc := range []struct {
		method, target string
		want           int
	}{
		{"GET", "/about", 200}, {"HEAD", "/about", 200}, {"GET", "/agents.txt", 200}, {"GET", "/missing", 404}, {"POST", "/about", 405},
	} {
		r := req(tc.method, tc.target, "", "")
		if r.Code != tc.want {
			t.Errorf("%s %s=%d want %d", tc.method, tc.target, r.Code, tc.want)
		}
		if !strings.Contains(r.Header().Get("Bashy-Shaped"), `module="github.com/fixture/site"`) || strings.HasPrefix(r.Header().Get("Bashy-Shaped"), "X-") {
			t.Errorf("%s %s Bashy-Shaped=%q", tc.method, tc.target, r.Header().Get("Bashy-Shaped"))
		}
		for _, rel := range []string{`rel="alternate"`, `rel="describedby"`, `rel="edit"`, `rel="via"`, `rel="service-doc"`, `rel="service-desc"`, `rel="canonical"`, `rel="author"`, `rel="help"`} {
			if !strings.Contains(r.Header().Get("Link"), rel) {
				t.Errorf("%s %s Link missing %s: %q", tc.method, tc.target, rel, r.Header().Get("Link"))
			}
		}
	}
	if r := req("GET", "/missing", "", ""); !strings.Contains(r.Body.String(), `id="agent-note"`) {
		t.Errorf("404 missing agent note")
	}
	if r := req("POST", "/about", "", ""); !strings.Contains(r.Body.String(), `id="agent-note"`) {
		t.Errorf("405 missing agent note")
	}
	// HTML body channels
	html := req("GET", "/about", "text/html", "").Body.String()
	for _, want := range []string{`<script id="agent-recipe" type="text/bashsharp">`, `<meta name="agent:instructions"`, `<meta name="generator" content="bashy site 0.2.0">`, `<p id="agent-note" role="note">`, `<section id="agents">`, `<div hidden>`, `<noscript>`, `<link rel="edit"`} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %s", want)
		}
	}
	for _, want := range []string{`"@type":"WebSite"`, "SoftwareSourceCode", "codeRepository", "https://github.com/fixture/site", "bashy fetch https://fixture.example/agents.txt", "bashy fetch https://fixture.example/about.md", "# Next: bashy site attach https://fixture.example"} {
		// JSON-LD is marshaled with spaces after colons; normalize for type check
		if want == `"@type":"WebSite"` {
			if !strings.Contains(html, "WebSite") { t.Errorf("html missing %s", want) }
			continue
		}
		if !strings.Contains(html, want) {
			t.Errorf("html missing %s", want)
		}
	}
	if strings.Index(html, `<script id="agent-recipe"`) > strings.Index(html, `id="agent-note"`) {
		t.Errorf("recipe should precede note in body start ordering check")
	}
	// MD first block + front matter, JSON agents first
	md := req("GET", "/about", "text/markdown", "").Body.String()
	if !strings.Contains(md, "agent:\n  instructions:") || !strings.HasPrefix(strings.TrimSpace(stripFrontMatter(md)), "## For agents {#agents}") {
		t.Errorf("md agent block not first:\n%s", md[:400])
	}
	js := req("GET", "/about", "application/json", "").Body.String()
	if !strings.HasPrefix(strings.TrimSpace(js), "{\n  \"agents\":") {
		t.Errorf("json agents not first: %.80s", js)
	}
	var jm map[string]json.RawMessage
	if err := json.Unmarshal([]byte(js), &jm); err != nil || jm["agents"] == nil {
		t.Errorf("json agents field: %v", err)
	}
	// Tie-break + agent UA
	if NegotiateAccept("text/markdown, text/html") != "md" || NegotiateAccept("text/html, text/markdown") != "html" || NegotiateAccept("*/*") != "html" {
		t.Errorf("tie-break failed")
	}
	if r := req("GET", "/about", "", "Claude-User (claude-code)"); !strings.HasPrefix(r.Header().Get("Content-Type"), "text/markdown") {
		t.Errorf("agent UA should get markdown, got %q", r.Header().Get("Content-Type"))
	}
	if r := req("GET", "/about", "", "Mozilla/5.0"); !strings.HasPrefix(r.Header().Get("Content-Type"), "text/html") {
		t.Errorf("browser UA should get html")
	}
	// Root + well-known
	for path, want := range map[string]string{
		"/AGENTS.md": "## For agents", "/skill.md": "fixture-site", "/llms.txt": "## For agents",
		"/.well-known/agent-skills/index.json": `"version": "0.2.0"`,
		"/.well-known/mcp/server-card.json":    `"schema_version": "mcp-server-card-v1"`,
		"/.well-known/oauth-protected-resource": `"resource": "https://fixture.example/mcp"`,
		"/.well-known/agent-card.json":         "fixture-site",
		"/.well-known/api-catalog":             "linkset",
		"/.well-known/security.txt":            "Policy:",
		"/robots.txt":                          "Sitemap:",
		"/sitemap.xml":                         "/about.md",
		"/feed.xml":                            "For agents",
		"/humans.txt":                          "Agents",
	} {
		r := req("GET", path, "", "")
		if r.Code != 200 || !strings.Contains(r.Body.String(), want) {
			t.Errorf("%s = %d missing %q", path, r.Code, want)
		}
	}
	// Bashy-Shaped derives skill/mcp/shape from config (same-origin stripped, external kept)
	loaded.Config.Skill = "https://fixture.example/custom-skill.md"
	loaded.Config.MCP = "https://mcp.example.net/mcp"
	loaded.Config.Shape = "https://fixture.example/custom-shape"
	if got := loaded.BashyShapedValue(loaded.ByPath["/about"]); !strings.Contains(got, `skill="/custom-skill.md"`) || !strings.Contains(got, `mcp="https://mcp.example.net/mcp"`) || !strings.Contains(got, `shape="/custom-shape"`) {
		t.Errorf("Bashy-Shaped configured paths: %s", got)
	}
	loaded.Config.Skill = "https://fixture.example/skill.md"
	loaded.Config.MCP = "https://fixture.example/mcp"
	loaded.Config.Shape = "https://fixture.example/?shape=open"
	_ = loaded
	if err := Validate(loaded); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
