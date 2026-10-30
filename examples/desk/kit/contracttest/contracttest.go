// Package contracttest is the shared desk contract test every tool must
// pass. It probes a running tool server over HTTP, so it is independent of
// each tool's storage and item model: the only coupling is the contract
// itself (verbs, shapes, headers, page markers).
//
// Use it from a tool's Go test:
//
//	func TestDeskContract(t *testing.T) {
//		srv := startYourServer(t)
//		contracttest.Run(t, contracttest.Config{BaseURL: srv.URL, Tool: "mytool"})
//	}
//
// Check returns one line per violated contract point (empty = green) so a
// meta-test can also assert that a deliberately broken server goes red.
package contracttest

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	kit "desk-kit"
)

// Config describes the tool server under test.
type Config struct {
	// BaseURL is the tool's loopback origin, e.g. http://127.0.0.1:18097
	// (optionally with a -prefix path). It must be loopback: new desk
	// tools bind 127.0.0.1 and reach the LAN only through sharing.
	BaseURL string
	// Tool is the tool name (data dir, CLI, launcher tile).
	Tool string
	// ItemParam is the query parameter naming an item in URLs ("id" ...).
	// Empty means "id".
	ItemParam string
	// StateHasContent records the retrofit exception for tools whose
	// GET /api/state is the page's full state (whiteboard, brainstorm):
	// the state read may carry item content, so the contentless-state
	// probe is skipped. open/new/list stay contentless regardless.
	StateHasContent bool
}

// Run fails the test once per violated contract point.
func Run(t *testing.T, c Config) {
	t.Helper()
	for _, f := range Check(c) {
		t.Errorf("desk contract: %s", f)
	}
}

func (c Config) param() string {
	if c.ItemParam != "" {
		return c.ItemParam
	}
	return "id"
}

// Check probes every contract point and returns failure lines.
func Check(cfg Config) (failures []string) {
	t := &check{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
	t.health()
	t.loopback()
	t.securityHeaders()
	t.pageKit()
	t.methods()
	t.crossSite()
	t.strictDecoding()
	t.names()
	t.lifecycle()
	t.secrets()
	return t.failures
}

type check struct {
	cfg      Config
	client   *http.Client
	failures []string
}

func (t *check) fail(format string, args ...any) {
	t.failures = append(t.failures, fmt.Sprintf(format, args...))
}

func (t *check) get(path string, headers map[string]string) (int, []byte, http.Header) {
	req, _ := http.NewRequest("GET", t.cfgBase()+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r, err := t.client.Do(req)
	if err != nil {
		t.fail("GET %s: %v", path, err)
		return 0, nil, nil
	}
	defer r.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	return r.StatusCode, body, r.Header
}

func (t *check) post(path string, body string, headers map[string]string) (int, []byte, http.Header) {
	req, _ := http.NewRequest("POST", t.cfgBase()+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r, err := t.client.Do(req)
	if err != nil {
		t.fail("POST %s: %v", path, err)
		return 0, nil, nil
	}
	defer r.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	return r.StatusCode, raw, r.Header
}

func (t *check) cfgBase() string { return strings.TrimSuffix(t.cfg.BaseURL, "/") }

func (t *check) method(m, path string) int {
	req, _ := http.NewRequest(m, t.cfgBase()+path, nil)
	r, err := t.client.Do(req)
	if err != nil {
		t.fail("%s %s: %v", m, path, err)
		return 0
	}
	defer r.Body.Close()
	io.Copy(io.Discard, io.LimitReader(r.Body, 1<<16))
	return r.StatusCode
}

func decode(t *check, body []byte) map[string]any {
	var v map[string]any
	if err := json.Unmarshal(body, &v); err != nil {
		t.fail("response is not a JSON object: %q", trunc(string(body), 120))
		return nil
	}
	return v
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// 1. Two front doors, human door first: the page and the health probe.
func (t *check) health() {
	code, body, _ := t.get("/healthz", nil)
	if code != http.StatusOK {
		t.fail("GET /healthz: want 200, got %d", code)
		return
	}
	var v struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &v); err != nil || !v.OK {
		t.fail("GET /healthz: want {\"ok\":true}, got %q", trunc(string(body), 80))
	}
}

// LoopbackViolation is the pure loopback-origin rule, unit-testable without
// a server: new tools bind loopback; the LAN is reached only through sharing.
func LoopbackViolation(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Sprintf("BaseURL %q does not parse: %v", baseURL, err)
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Sprintf("BaseURL host %q is not loopback; desk tools must bind 127.0.0.1/::1", host)
	}
	return ""
}

// New tools bind loopback; the LAN is reached only through sharing.
func (t *check) loopback() {
	if v := LoopbackViolation(t.cfg.BaseURL); v != "" {
		t.fail("%s", v)
	}
}

// Security headers ride on every response, page and API alike.
func (t *check) securityHeaders() {
	for _, path := range []string{"/", "/healthz", "/api/state?" + t.cfg.param() + "=no-such-item"} {
		_, _, h := t.get(path, nil)
		if h == nil {
			continue
		}
		if h.Get("Cache-Control") != "no-store" {
			t.fail("GET %s: Cache-Control must be no-store", path)
		}
		if h.Get("X-Content-Type-Options") != "nosniff" {
			t.fail("GET %s: X-Content-Type-Options must be nosniff", path)
		}
		if !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.fail("GET %s: Content-Security-Policy must forbid framing", path)
		}
	}
}

// The human page carries the UI kit: copy button + copied state, status
// badges, field components, item header, light/dark theme.
func (t *check) pageKit() {
	code, body, h := t.get("/", nil)
	if code != http.StatusOK {
		t.fail("GET /: want 200, got %d", code)
		return
	}
	if ct := h.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.fail("GET /: want text/html, got %q", ct)
	}
	page := string(body)
	for _, marker := range []string{
		kit.MarkerCopyButton, "Copied", kit.MarkerBadge,
		kit.MarkerItemHeader, kit.MarkerField, "prefers-color-scheme",
	} {
		if !strings.Contains(page, marker) {
			t.fail("GET /: page lacks UI-kit marker %q", marker)
		}
	}
}

// Only GET and POST exist.
func (t *check) methods() {
	for _, m := range []string{"PUT", "DELETE", "PATCH"} {
		if got := t.method(m, "/api/action"); got != http.StatusMethodNotAllowed {
			t.fail("%s /api/action: want 405, got %d", m, got)
		}
	}
}

// Cross-site posts are refused.
func (t *check) crossSite() {
	code, _, _ := t.post("/api/action", `{"action":"list"}`,
		map[string]string{"Sec-Fetch-Site": "cross-site"})
	if code != http.StatusForbidden {
		t.fail("cross-site POST: want 403, got %d", code)
	}
	code, _, _ = t.post("/api/action", `{"action":"list"}`,
		map[string]string{"Origin": "https://evil.example"})
	if code != http.StatusForbidden {
		t.fail("foreign-Origin POST: want 403, got %d", code)
	}
}

// Strict decoding: unknown fields, trailing garbage and unknown actions
// are all 400.
func (t *check) strictDecoding() {
	if code, _, _ := t.post("/api/action", `{"action":"list","bogus":1}`, nil); code != http.StatusBadRequest {
		t.fail("unknown field: want 400, got %d", code)
	}
	if code, _, _ := t.post("/api/action", `{"action":"list"} trailing`, nil); code != http.StatusBadRequest {
		t.fail("trailing garbage: want 400, got %d", code)
	}
	if code, _, _ := t.post("/api/action", `{"action":"frobnicate"}`, nil); code != http.StatusBadRequest {
		t.fail("unknown action: want 400, got %d", code)
	}
	if code, _, _ := t.post("/api/action", `not json`, nil); code != http.StatusBadRequest {
		t.fail("malformed JSON: want 400, got %d", code)
	}
}

// Bad ids are 400, unknown ids are 404 and never mint storage.
func (t *check) names() {
	p := t.cfg.param()
	if code, _, _ := t.get("/api/state?"+p+"=bad!!name", nil); code != http.StatusBadRequest {
		t.fail("bad id over GET: want 400, got %d", code)
	}
	if code, _, _ := t.get("/api/state?"+p+"=no-such-item", nil); code != http.StatusNotFound {
		t.fail("unknown id over GET: want 404, got %d", code)
	}
	if code, _, _ := t.post("/api/action", `{"action":"open","id":"bad!!name"}`, nil); code != http.StatusBadRequest {
		t.fail("bad id over POST: want 400, got %d", code)
	}
	if code, _, _ := t.post("/api/action", `{"action":"get","id":"no-such-item"}`, nil); code != http.StatusNotFound {
		t.fail("unknown id over POST: want 404, got %d", code)
	}
}

// Full item lifecycle through the agent door: new → open (contentless) →
// get (content) → put → list (contentless) → share → close.
func (t *check) lifecycle() {
	canary := "canary-content-marker"
	newBody := fmt.Sprintf(`{"action":"new","title":"contract probe","body":%q}`, canary)
	code, raw, _ := t.post("/api/action", newBody, nil)
	if code != http.StatusOK {
		t.fail("new: want 200, got %d", code)
		return
	}
	created := decode(t, raw)
	if created == nil {
		return
	}
	id, _ := created["id"].(string)
	itemURL, _ := created["url"].(string)
	status, _ := created["status"].(string)
	if !kit.ValidName(id) {
		t.fail("new: id %q is not a stable desk id", id)
		return
	}
	if itemURL == "" || !strings.Contains(itemURL, id) {
		t.fail("new: url %q must be stable and contain the id", itemURL)
	}
	if !kit.ValidStatus(status) {
		t.fail("new: status %q is not a known desk status", status)
	}
	if strings.Contains(string(raw), canary) {
		t.fail("new: response re-emits content; new/open must return id+url+status only")
	}
	key, _ := created["ownerKey"].(string)
	auth := map[string]string{}
	if key != "" {
		auth["X-Facilitator-Key"] = key
	}

	// open: same stable triple, never content.
	code, raw, _ = t.post("/api/action", fmt.Sprintf(`{"action":"open","id":%q}`, id), auth)
	if code != http.StatusOK {
		t.fail("open: want 200, got %d", code)
		return
	}
	opened := decode(t, raw)
	if opened == nil {
		return
	}
	if opened["id"] != id || opened["url"] != itemURL {
		t.fail("open: id/url must be stable across re-surface (got %v)", opened)
	}
	if strings.Contains(string(raw), canary) {
		t.fail("open: response re-emits content; open must return id+url+status only")
	}

	// GET /api/state is the same contentless re-surface.
	p := t.cfg.param()
	code, raw, _ = t.get("/api/state?"+p+"="+url.QueryEscape(id), auth)
	if code != http.StatusOK {
		t.fail("GET /api/state: want 200, got %d", code)
	} else if !t.cfg.StateHasContent && strings.Contains(string(raw), canary) {
		t.fail("GET /api/state: must not re-emit content")
	}

	// get: the one read that returns content.
	code, raw, _ = t.post("/api/action", fmt.Sprintf(`{"action":"get","id":%q}`, id), auth)
	if code != http.StatusOK {
		t.fail("get: want 200, got %d", code)
	} else if !strings.Contains(string(raw), canary) {
		t.fail("get: response must carry the item content")
	}

	// put: content changes, secrets aside (covered separately).
	code, raw, _ = t.post("/api/action",
		fmt.Sprintf(`{"action":"put","id":%q,"body":"updated probe body"}`, id), auth)
	if code != http.StatusOK {
		t.fail("put: want 200, got %d", code)
	}

	// list: ids + statuses, never content.
	code, raw, _ = t.post("/api/action", `{"action":"list"}`, auth)
	if code != http.StatusOK {
		t.fail("list: want 200, got %d", code)
	} else {
		if strings.Contains(string(raw), canary) || strings.Contains(string(raw), "updated probe body") {
			t.fail("list: must not re-emit content")
		}
		var listed []map[string]any
		if err := json.Unmarshal(raw, &listed); err != nil {
			// Some tools wrap: {"items": [...]}.
			var wrapped struct {
				Items []map[string]any `json:"items"`
			}
			if werr := json.Unmarshal(raw, &wrapped); werr != nil || wrapped.Items == nil {
				t.fail("list: want a JSON array of {id,status}, got %q", trunc(string(raw), 120))
			} else {
				listed = wrapped.Items
			}
		}
		found := false
		for _, e := range listed {
			if e["id"] == id {
				found = true
			}
		}
		if !found {
			t.fail("list: created item %q missing", id)
		}
	}

	// Duplicate new with the same id collides.
	code, _, _ = t.post("/api/action",
		fmt.Sprintf(`{"action":"new","id":%q,"title":"dup"}`, id), auth)
	if code != http.StatusConflict {
		t.fail("duplicate new: want 409, got %d", code)
	}

	// share mints a single-use invite; close ends the item.
	code, raw, _ = t.post("/api/action", fmt.Sprintf(`{"action":"share","id":%q}`, id), auth)
	if code != http.StatusOK {
		t.fail("share: want 200, got %d", code)
	} else {
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		if _, ok := v["invite"].(string); !ok {
			t.fail("share: response must carry an invite code")
		}
	}
	code, raw, _ = t.post("/api/action", fmt.Sprintf(`{"action":"close","id":%q}`, id), auth)
	if code != http.StatusOK {
		t.fail("close: want 200, got %d", code)
	} else {
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		if v["status"] != kit.StatusClosed {
			t.fail("close: status must become %q, got %v", kit.StatusClosed, v["status"])
		}
	}
	code, _, _ = t.post("/api/action",
		fmt.Sprintf(`{"action":"put","id":%q,"body":"after close"}`, id), auth)
	if code != http.StatusConflict {
		t.fail("put after close: want 409, got %d", code)
	}
}

// Secrets never flow through desk: a secret-typed field (or a
// secret-named one) is refused before storage.
func (t *check) secrets() {
	for _, body := range []string{
		`{"action":"new","title":"s","fields":[{"name":"notes","type":"secret","value":"hunter2"}]}`,
		`{"action":"new","title":"s","fields":[{"name":"db_password","type":"text","value":"hunter2"}]}`,
	} {
		code, raw, _ := t.post("/api/action", body, nil)
		if code != http.StatusBadRequest {
			t.fail("secret field %s: want 400, got %d", trunc(body, 60), code)
			continue
		}
		if !strings.Contains(strings.ToLower(string(raw)), "secret") {
			t.fail("secret refusal must name the reason; got %q", trunc(string(raw), 80))
		}
	}
}
