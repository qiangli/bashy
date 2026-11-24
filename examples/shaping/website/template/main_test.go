package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

const testSecret = "test-secret"

// fixture is a fixed asset tree, so these tests never depend on the owner's
// own site.json or fields.
var fixture = fstest.MapFS{
	"site.json": {Data: []byte(`{"site_url":"https://example.com","title":"Example Person","contact":"mailto:owner@example.com",
	  "ai":{"base_url":"http://127.0.0.1:1","model":"m","key_env":"SITE_AI_KEY","daily_cap":5,"daily_max":50},
	  "shape_url":"/matrix/h/host/app/apps/shape/site-shaping/",
	  "views":[{"name":"recruiter-summary","title":"Recruiter summary","prompt":"Summarize.","audience":{"account":true}}],
	  "properties":{
	    "theme":{"values":["default","dark","light"],"class_prefix":"ss-theme-"},
	    "text_size":{"values":["default","large","larger"],"class_prefix":"ss-text-"},
	    "density":{"values":["default","compact"],"class_prefix":"ss-density-"},
	    "contrast":{"values":["default","high"],"class_prefix":"ss-contrast-"},
	    "motion":{"values":["default","reduce"],"class_prefix":"ss-motion-"}
	  },
	  "wishes":[
	    {"id":"dark","label":"Dark","patch":{"theme":"dark"}},
	    {"id":"light","label":"Light","patch":{"theme":"light"}},
	    {"id":"larger-text","label":"Larger text","patch":{"text_size":"large"}},
	    {"id":"compact","label":"Compact","patch":{"density":"compact"}},
	    {"id":"high-contrast","label":"High contrast","patch":{"contrast":"high"}},
	    {"id":"reduce-motion","label":"Reduce motion","patch":{"motion":"reduce"}}
	  ]}`)},
	"fields/contact.md":      {Data: []byte("---\nlabel: Contact\ncircles: recruiter\n---\nEmail: owner@example.com\n")},
	"fields/certificates.md": {Data: []byte("---\nlabel: Certificates\ncircles: recruiter\n---\nExample Certified Professional\n")},
	"fields/notes.md":        {Data: []byte("---\nlabel: Owner notes\n---\nowner only\n")},
	"public/shaping.js":      {Data: []byte("//")},
}

// stamp signs headers the way outpost does (version-2 relationship payload).
func stamp(r *http.Request, kind, circle, groups, visitor, user string, now time.Time) {
	prefix := "/matrix/h/host/app/site"
	ts := strconv.FormatInt(now.Unix(), 10)
	r.Header.Set("X-Forwarded-Prefix", prefix)
	r.Header.Set("X-Outpost-Identity-Ts", ts)
	r.Header.Set("X-Outpost-Relationship", kind)
	if circle != "" {
		r.Header.Set("X-Outpost-Circle", circle)
	}
	if groups != "" {
		r.Header.Set("X-Outpost-Groups", groups)
	}
	if visitor != "" {
		r.Header.Set("X-Outpost-Visitor", visitor)
	}
	if user != "" {
		r.Header.Set("Remote-User", user)
	}
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte("v2\n" + kind + "\n" + circle + "\n" + groups + "\n" + visitor + "\n" + user + "\n" + prefix + "\n" + ts))
	r.Header.Set("X-Outpost-Relationship-Sig", hex.EncodeToString(mac.Sum(nil)))
}

type who struct {
	name                                string
	kind, circle, groups, visitor, user string
}

var (
	owner          = who{"owner", "owner", "", "", "", "owner@example.com"}
	recruiter      = who{"recruiter", "member", "recruiter", "", "", "rec@example.com"}
	friend         = who{"friend", "member", "friend", "", "", "pal@example.com"}
	stranger       = who{"account", "account", "", "", "0123456789abcdef0123456789abcdef", ""}
	strangerLegacy = who{"signed-in alias", "signed-in", "", "", "0123456789abcdef0123456789abcdef", ""}
	nobody         = who{"anonymous", "", "", "", "", ""}
)

func newTestServer(t *testing.T, site, model string, key string) *server {
	t.Helper()
	env := map[string]string{"SITE_SSO_SECRET": testSecret, "SITE_AI_KEY": key}
	s, err := newServerFrom(fixture, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	s.site = newUpstream(site)
	s.cfg.AI.BaseURL = model
	return s
}

func do(t *testing.T, s *server, method, path string, w who) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if w.kind != "" {
		stamp(req, w.kind, w.circle, w.groups, w.visitor, w.user, time.Now())
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func fieldNames(out map[string]any) []string {
	var names []string
	list, _ := out["fields"].([]any)
	for _, f := range list {
		names = append(names, f.(map[string]any)["name"].(string))
	}
	return names
}

func TestTierMatrix(t *testing.T) {
	s := newTestServer(t, "http://127.0.0.1:1", "http://127.0.0.1:1", "k")
	for _, tc := range []struct {
		w      who
		tier   string
		fields string
		views  int
	}{
		{owner, "owner", "certificates,contact,notes", 1},
		{recruiter, "member", "certificates,contact", 1},
		{friend, "member", "", 1},
		{stranger, "account", "", 1},
		{strangerLegacy, "account", "", 1},
		{nobody, "anonymous", "", 0},
	} {
		_, me := do(t, s, "GET", "/_shaping/api/me", tc.w)
		if me["tier"] != tc.tier {
			t.Errorf("%s: tier %v, want %s", tc.w.name, me["tier"], tc.tier)
		}
		views, _ := me["views"].([]any)
		if len(views) != tc.views {
			t.Errorf("%s: %d views, want %d", tc.w.name, len(views), tc.views)
		}
		_, f := do(t, s, "GET", "/_shaping/api/fields", tc.w)
		if got := strings.Join(fieldNames(f), ","); got != tc.fields {
			t.Errorf("%s: fields %q, want %q", tc.w.name, got, tc.fields)
		}
	}
}

func TestForgedOrStaleStampIsAnonymous(t *testing.T) {
	s := newTestServer(t, "http://127.0.0.1:1", "http://127.0.0.1:1", "k")
	// Unsigned headers claiming owner.
	req := httptest.NewRequest("GET", "/_shaping/api/fields", nil)
	req.Header.Set("X-Forwarded-Prefix", "/p")
	req.Header.Set("X-Outpost-Relationship", "owner")
	req.Header.Set("Remote-User", "owner@example.com")
	if c := resolveCaller(req, testSecret, time.Now()); c.Tier != TierAnonymous {
		t.Errorf("unsigned owner claim resolved to %q", c.Tier)
	}
	// Valid signature, then tampered circle.
	req = httptest.NewRequest("GET", "/", nil)
	stamp(req, "member", "friend", "", "", "pal@example.com", time.Now())
	req.Header.Set("X-Outpost-Circle", "recruiter")
	if c := resolveCaller(req, testSecret, time.Now()); c.Tier != TierAnonymous {
		t.Errorf("tampered circle resolved to %q", c.Tier)
	}
	// Stale.
	req = httptest.NewRequest("GET", "/", nil)
	stamp(req, "owner", "", "", "", "owner@example.com", time.Now().Add(-5*time.Minute))
	if c := resolveCaller(req, testSecret, time.Now()); c.Tier != TierAnonymous {
		t.Errorf("stale stamp resolved to %q", c.Tier)
	}
	// No secret configured: nothing is trusted.
	req = httptest.NewRequest("GET", "/", nil)
	stamp(req, "owner", "", "", "", "owner@example.com", time.Now())
	if c := resolveCaller(req, "", time.Now()); c.Tier != TierAnonymous {
		t.Errorf("no secret resolved to %q", c.Tier)
	}
	_ = s
}

func TestAudienceFailsClosed(t *testing.T) {
	var a Audience
	if a.Admits(Caller{Tier: TierMember, Circle: "recruiter", Email: "x@example.com"}) {
		t.Error("empty audience admitted a member")
	}
	if !a.Admits(Caller{Tier: TierOwner}) {
		t.Error("owner must see everything")
	}
	g := Audience{Circles: []string{"recruiter"}, Groups: []string{"hiring"}}
	if g.Admits(Caller{Tier: TierMember, Circle: "friend", Groups: []string{"hiring"}}) {
		t.Error("a group must never widen across circles")
	}
	if !g.Admits(Caller{Tier: TierMember, Circle: "recruiter", Groups: []string{"hiring"}}) {
		t.Error("circle and group member refused")
	}
	if (Audience{Circles: []string{"recruiter"}}).Admits(Caller{Tier: TierAccount, Visitor: "v"}) {
		t.Error("a stranger must not match a circle")
	}
}

func TestSignedInWireAndAudienceAliases(t *testing.T) {
	// Legacy wire value "signed-in" resolves to TierAccount and logs a deprecation notice.
	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	req := httptest.NewRequest("GET", "/", nil)
	now := time.Now()
	stamp(req, "signed-in", "", "", "0123456789abcdef0123456789abcdef", "", now)
	if c := resolveCaller(req, testSecret, now); c.Tier != TierAccount || c.Visitor != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("signed-in alias resolved to %+v, want TierAccount", c)
	}
	if !strings.Contains(logBuf.String(), "signed-in") {
		t.Errorf("signed-in alias did not log warning: %q", logBuf.String())
	}

	// Canonical "account" wire value resolves to TierAccount without logging an alias warning.
	logBuf.Reset()
	req = httptest.NewRequest("GET", "/", nil)
	stamp(req, "account", "", "", "0123456789abcdef0123456789abcdef", "", now)
	if c := resolveCaller(req, testSecret, now); c.Tier != TierAccount || c.Visitor != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("account stamp resolved to %+v, want TierAccount", c)
	}
	if logBuf.Len() != 0 {
		t.Errorf("canonical account stamp logged unexpected warning: %q", logBuf.String())
	}

	// Legacy config key "signed_in" aliases "account" in both site.json and fields/*.md.
	legacyFS := fstest.MapFS{
		"site.json": {Data: []byte(`{"site_url":"https://example.com","title":"T",
		  "ai":{"base_url":"http://127.0.0.1:1","model":"m","key_env":"SITE_AI_KEY"},
		  "views":[{"name":"legacy-view","title":"Legacy","prompt":"p","audience":{"signed_in":true}}]}`)},
		"fields/legacy-fact.md":  {Data: []byte("---\nlabel: Legacy\nsigned_in: true\n---\nlegacy body\n")},
		"fields/account-fact.md": {Data: []byte("---\nlabel: Account\naccount: true\n---\naccount body\n")},
		"public/shaping.js":      {Data: []byte("//")},
	}
	env := map[string]string{"SITE_SSO_SECRET": testSecret, "SITE_AI_KEY": "k"}
	s, err := newServerFrom(legacyFS, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []who{stranger, strangerLegacy} {
		_, me := do(t, s, "GET", "/_shaping/api/me", w)
		if me["tier"] != TierAccount {
			t.Errorf("%s: tier = %v, want %s", w.name, me["tier"], TierAccount)
		}
		if views, _ := me["views"].([]any); len(views) != 1 {
			t.Errorf("%s: views = %v, want 1", w.name, views)
		}
		_, f := do(t, s, "GET", "/_shaping/api/fields", w)
		if got := strings.Join(fieldNames(f), ","); got != "account-fact,legacy-fact" {
			t.Errorf("%s: fields = %q, want account-fact,legacy-fact", w.name, got)
		}
	}
	_, f := do(t, s, "GET", "/_shaping/api/fields", nobody)
	if got := strings.Join(fieldNames(f), ","); got != "" {
		t.Errorf("anonymous saw fields %q", got)
	}
}

func TestProxyRebasesAndInjects(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<html><head><link href="/_next/static/a.css" rel="stylesheet"><link rel="icon" href="/favicon.ico"></head><body><a href="/about">x</a><img src="//cdn.example/i.png"><script>self.__next_f.push(["\"/_next/static/c.js\""])</script></body></html>`)
	}))
	defer site.Close()
	s := newTestServer(t, site.URL, "http://127.0.0.1:1", "")
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Prefix", "/matrix/h/host/app/site")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		`href="` + site.URL + `/_next/static/a.css"`,
		`href="/matrix/h/host/app/site/about"`,
		`<link rel="icon" href="` + site.URL + `/favicon.ico">`,
		`src="//cdn.example/i.png"`,
		`\"` + site.URL + `/_next/static/c.js\"`,
		`<script src="/matrix/h/host/app/site/_shaping/shaping.js" defer></script></body>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in\n%s", want, body)
		}
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("engine pages must not be cached")
	}
}

func TestViewUsesOnlyAllowedDataAndCaps(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<html><body><h1>Public Résumé</h1><p>Engineer.</p></body></html>`)
	}))
	defer site.Close()
	var prompts []string
	var auth string
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		prompts = append(prompts, string(b))
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"## Summary\n- Engineer"}}]}`)
	}))
	defer model.Close()
	s := newTestServer(t, site.URL, model.URL, "free-key")
	s.cfg.AI.DailyCap = 2

	code, out := do(t, s, "POST", "/_shaping/api/view?name=recruiter-summary", stranger)
	if code != http.StatusOK || !strings.Contains(out["markdown"].(string), "Engineer") {
		t.Fatalf("stranger view: %d %v", code, out)
	}
	if auth != "Bearer free-key" {
		t.Errorf("route key not sent: %q", auth)
	}
	if strings.Contains(prompts[0], "owner@example.com") || strings.Contains(prompts[0], "Certified") || strings.Contains(prompts[0], "Owner notes") {
		t.Errorf("stranger prompt carries private data:\n%s", prompts[0])
	}
	if !strings.Contains(prompts[0], "Public Résumé") {
		t.Errorf("prompt lacks the public page text:\n%s", prompts[0])
	}

	do(t, s, "POST", "/_shaping/api/view?name=recruiter-summary", recruiter)
	if last := prompts[len(prompts)-1]; !strings.Contains(last, "Certified") || strings.Contains(last, "Owner notes") {
		t.Errorf("recruiter prompt must carry recruiter facts only:\n%s", last)
	}

	do(t, s, "POST", "/_shaping/api/view?name=recruiter-summary", stranger)
	if code, _ := do(t, s, "POST", "/_shaping/api/view?name=recruiter-summary", stranger); code != http.StatusTooManyRequests {
		t.Errorf("third view for the same visitor: %d, want 429", code)
	}
	if code, _ := do(t, s, "POST", "/_shaping/api/view?name=recruiter-summary", nobody); code != http.StatusForbidden {
		t.Errorf("anonymous view: %d, want 403", code)
	}
}

func TestNoRouteNoViews(t *testing.T) {
	s := newTestServer(t, "http://127.0.0.1:1", "https://models.example/v1", "")
	_, me := do(t, s, "GET", "/_shaping/api/me", stranger)
	if views, _ := me["views"].([]any); len(views) != 0 {
		t.Errorf("views offered without a key: %v", views)
	}
}

func TestStaticOnlyStripsFrameworkScripts(t *testing.T) {
	page := `<html><head><link rel="preload" as="script" href="/_next/static/chunks/a.js"><script>document.documentElement.classList.add("dark")</script></head><body><h1>Hi</h1><script src="/_next/static/chunks/main.js" async></script><script>self.__next_f.push([1,"x"])</script></body></html>`
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, page)
	}))
	defer site.Close()
	s := newTestServer(t, site.URL, "http://127.0.0.1:1", "")
	s.site.staticOnly = true
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Prefix", "/p")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, gone := range []string{"main.js", "__next_f", `as="script"`} {
		if strings.Contains(body, gone) {
			t.Errorf("%s survived:\n%s", gone, body)
		}
	}
	for _, kept := range []string{`classList.add("dark")`, "<h1>Hi</h1>", "/p/_shaping/shaping.js"} {
		if !strings.Contains(body, kept) {
			t.Errorf("%s missing:\n%s", kept, body)
		}
	}
}

func TestShapeEntryIsOwnerOnly(t *testing.T) {
	s := newTestServer(t, "http://127.0.0.1:1", "http://127.0.0.1:1", "k")
	for _, w := range []who{owner, recruiter, stranger, nobody} {
		_, me := do(t, s, "GET", "/_shaping/api/me", w)
		got, _ := me["shape"].(string)
		if (w.name == "owner") != (got != "") {
			t.Errorf("%s: shape = %q", w.name, got)
		}
	}
}

func TestMeSaysWhetherVisitorsAreIdentified(t *testing.T) {
	s := newTestServer(t, "http://127.0.0.1:1", "http://127.0.0.1:1", "")
	if _, me := do(t, s, "GET", "/_shaping/api/me", nobody); me["public"] != true {
		t.Errorf("with a secret: public = %v", me["public"])
	}
	s.secret = ""
	if _, me := do(t, s, "GET", "/_shaping/api/me", nobody); me["public"] != false {
		t.Errorf("without a secret: public = %v", me["public"])
	}
}

func TestAnonymousGetsWishesWithoutModelCall(t *testing.T) {
	modelCalls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls++
		http.Error(w, "unexpected model call", http.StatusInternalServerError)
	}))
	defer model.Close()

	// Even without an AI key, anonymous visitors get the curated wishes list.
	s := newTestServer(t, "http://127.0.0.1:1", model.URL, "")
	code, me := do(t, s, "GET", "/_shaping/api/me", nobody)
	if code != http.StatusOK {
		t.Fatalf("GET me: %d", code)
	}
	if views, _ := me["views"].([]any); len(views) != 0 {
		t.Errorf("anonymous views = %v, want empty", views)
	}
	wishes, _ := me["wishes"].([]any)
	if len(wishes) != 6 {
		t.Fatalf("anonymous wishes len = %d, want 6 (%v)", len(wishes), wishes)
	}
	props, _ := me["properties"].(map[string]any)
	for _, k := range []string{"theme", "text_size", "density", "contrast", "motion"} {
		if _, ok := props[k]; !ok {
			t.Errorf("properties missing %q", k)
		}
	}
	if code, _ := do(t, s, "POST", "/_shaping/api/view?name=recruiter-summary", nobody); code != http.StatusForbidden {
		t.Errorf("anonymous view status = %d, want 403", code)
	}
	if modelCalls != 0 {
		t.Errorf("modelCalls = %d, want 0", modelCalls)
	}
}

func TestWishesFailClosedOnUndeclaredPropertyOrValue(t *testing.T) {
	props := map[string]Property{
		"theme":       {Values: []string{"default", "dark", "light"}, ClassPrefix: "ss-theme-"},
		"--ss-accent": {Values: []string{"#0f766e"}, CSSVar: "--ss-accent"},
		"empty_prop":  {},
	}
	if err := validateWish(Wish{ID: "dark", Label: "Dark", Patch: map[string]string{"theme": "dark"}}, props); err != nil {
		t.Errorf("valid class wish refused: %v", err)
	}
	if err := validateWish(Wish{ID: "teal", Label: "Teal", Patch: map[string]string{"--ss-accent": "#0f766e"}}, props); err != nil {
		t.Errorf("valid css var wish refused: %v", err)
	}
	if err := validateWish(Wish{ID: "bad-prop", Label: "Bad", Patch: map[string]string{"font_family": "Comic"}}, props); err == nil || !strings.Contains(err.Error(), "font_family") {
		t.Errorf("undeclared property error = %v, want error naming font_family", err)
	}
	if err := validateWish(Wish{ID: "bad-val", Label: "BadVal", Patch: map[string]string{"theme": "neon"}}, props); err == nil || !strings.Contains(err.Error(), "theme") {
		t.Errorf("disallowed value error = %v, want error naming theme", err)
	}
	if err := validateWish(Wish{ID: "empty-vals", Label: "Empty", Patch: map[string]string{"empty_prop": "x"}}, props); err == nil || !strings.Contains(err.Error(), "empty_prop") {
		t.Errorf("empty allowed values error = %v, want error naming empty_prop", err)
	}

	// Server /_shaping/api/me also drops invalid wishes fail-closed if site.json was hand-edited.
	dirty := fstest.MapFS{
		"site.json": {Data: []byte(`{"site_url":"https://example.com","title":"T",
		  "properties":{"theme":{"values":["default","dark"]}},
		  "wishes":[
		    {"id":"dark","label":"Dark","patch":{"theme":"dark"}},
		    {"id":"evil","label":"Evil","patch":{"custom_css":"red"}},
		    {"id":"neon","label":"Neon","patch":{"theme":"neon"}}
		  ]}`)},
		"public/shaping.js": {Data: []byte("//")},
	}
	s, err := newServerFrom(dirty, func(k string) string {
		if k == "SITE_SSO_SECRET" {
			return testSecret
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	_, me := do(t, s, "GET", "/_shaping/api/me", nobody)
	wishes, _ := me["wishes"].([]any)
	if len(wishes) != 1 || wishes[0].(map[string]any)["id"] != "dark" {
		t.Errorf("allowed wishes = %v, want only [dark]", wishes)
	}
}

func TestEmbeddedTemplateHasValidDefaultWishesAndAssets(t *testing.T) {
	s, err := newServer(func(k string) string {
		if k == "SITE_SSO_SECRET" {
			return testSecret
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.cfg.Wishes) != 6 {
		t.Fatalf("embedded template wishes = %d, want 6", len(s.cfg.Wishes))
	}
	if got := s.allowedWishes(); len(got) != 6 {
		t.Fatalf("embedded template allowedWishes = %d, want 6", len(got))
	}
	for _, tc := range []struct {
		path, want string
	}{
		{"/_shaping/shaping.js", "localStorage"},
		{"/_shaping/shaping.js", "renderWishes"},
		{"/_shaping/shaping.js", "renderShapeBox"},
		{"/_shaping/shaping.css", "ss-theme-dark"},
		{"/_shaping/shaping.css", "ss-motion-reduce"},
		{"/_shaping/shaping.css", "ss-shape-box"},
	} {
		req := httptest.NewRequest("GET", tc.path, nil)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: status=%d missing %q", tc.path, rec.Code, tc.want)
		}
	}
}

func TestShapedJSONScopesParser(t *testing.T) {
	// Default embedded shaped.json has server config and no scopes.
	rawDefault, err := fs.ReadFile(assets, "shaped.json")
	if err != nil {
		t.Fatalf("read embedded shaped.json: %v", err)
	}
	if got, err := parseShapedScopes(rawDefault); err != nil || !got.Edit.Empty() || !got.Shape.Empty() {
		t.Fatalf("default shaped.json = %+v, err=%v", got, err)
	}

	// Valid scopes with circles, groups and emails.
	valid := `{
	  "server": {"main": ".", "args": ["-addr", "$ADDR"]},
	  "scopes": {
	    "edit": {"circles": [" company "], "groups": ["support"], "emails": ["A@Example.com"]},
	    "shape": {"circles": ["contractor"], "groups": ["support"]}
	  }
	}`
	scopes, err := parseShapedScopes([]byte(valid))
	if err != nil {
		t.Fatalf("valid scopes refused: %v", err)
	}
	if len(scopes.Edit.Emails) != 1 || scopes.Edit.Emails[0] != "a@example.com" {
		t.Errorf("emails normalization = %v", scopes.Edit.Emails)
	}

	// Unknown scope names, non-member audience keys, and invalid values fail closed.
	for name, bad := range map[string]string{
		"unknown scope":        `{"scopes":{"admin":{"circles":["c"]}}}`,
		"view scope forbidden": `{"scopes":{"view":{"circles":["c"]}}}`,
		"account key":          `{"scopes":{"shape":{"account":true}}}`,
		"signed_in key":        `{"scopes":{"shape":{"signed_in":true}}}`,
		"unknown manifest key": `{"unknown_top":true}`,
		"empty circle":         `{"scopes":{"shape":{"circles":[""]}}}`,
		"space in circle":      `{"scopes":{"shape":{"circles":["bad circle"]}}}`,
		"empty group":          `{"scopes":{"edit":{"groups":[" "]}}}`,
		"bad email":            `{"scopes":{"edit":{"emails":["not-an-email"]}}}`,
		"trailing data":        `{"scopes":{}}{"extra":1}`,
	} {
		if _, err := parseShapedScopes([]byte(bad)); err == nil {
			t.Errorf("%s: expected error for %s", name, bad)
		}
	}

	// Live managed checkout (<home>/src/<app>/shaped.json) updates take effect immediately.
	home := t.TempDir()
	appDir := filepath.Join(home, "src", "site-shaping")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	shapedPath := filepath.Join(appDir, "shaped.json")
	if err := os.WriteFile(shapedPath, []byte(`{"server":{"args":["-addr","$ADDR"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"SITE_SSO_SECRET": testSecret,
		"SITE_SHAPE_APP":  "site-shaping,site",
		"SITE_SHAPE_HOME": home,
	}
	s, err := newServerFrom(fixture, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	memberC := who{"member-shape", "member", "contractor", "support", "", "c@example.com"}
	if _, me := do(t, s, "GET", "/_shaping/api/me", memberC); me["scope"] != "" {
		t.Fatalf("before scope grant: scope = %v, want empty", me["scope"])
	}
	if err := os.WriteFile(shapedPath, []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, me := do(t, s, "GET", "/_shaping/api/me", memberC); me["scope"] != ScopeShape {
		t.Fatalf("after scope grant: scope = %v, want %q", me["scope"], ScopeShape)
	}
	// Corrupt shaped.json on disk fails closed to empty scope.
	if err := os.WriteFile(shapedPath, []byte(`{"scopes":{"bogus":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, me := do(t, s, "GET", "/_shaping/api/me", memberC); me["scope"] != "" {
		t.Fatalf("corrupt shaped.json: scope = %v, want empty", me["scope"])
	}
}

func TestMeScopeMatrix(t *testing.T) {
	scopedFS := fstest.MapFS{}
	for k, v := range fixture {
		scopedFS[k] = v
	}
	scopedFS["shaped.json"] = &fstest.MapFile{Data: []byte(`{
	  "server": {"main": ".", "args": ["-addr", "$ADDR", "-data", "$DATA"]},
	  "scopes": {
	    "edit": {"circles": ["company"], "groups": ["support"]},
	    "shape": {"circles": ["contractor"], "groups": ["support"], "emails": ["lead@example.com"]}
	  }
	}`)}
	env := map[string]string{"SITE_SSO_SECRET": testSecret, "SITE_AI_KEY": "k"}
	s, err := newServerFrom(scopedFS, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}

	memberShape := who{"member with shape", "member", "contractor", "support", "", "c@example.com"}
	memberShapeByEmail := who{"member with shape by email", "member", "contractor", "", "", "lead@example.com"}
	memberEdit := who{"member with edit", "member", "company", "support", "", "a@example.com"}
	memberNoScope := who{"member without scope", "member", "company", "", "", "m@example.com"}
	memberWrongCircle := who{"member wrong circle", "member", "guest", "support", "", "g@example.com"}

	for _, tc := range []struct {
		w         who
		wantTier  string
		wantScope string
		wantShape bool
	}{
		{owner, TierOwner, ScopeShape, true},
		{memberShape, TierMember, ScopeShape, false},
		{memberShapeByEmail, TierMember, ScopeShape, false},
		{memberEdit, TierMember, ScopeEdit, false},
		{memberNoScope, TierMember, "", false},
		{memberWrongCircle, TierMember, "", false},
		{stranger, TierAccount, "", false},
		{strangerLegacy, TierAccount, "", false},
		{nobody, TierAnonymous, "", false},
	} {
		code, me := do(t, s, "GET", "/_shaping/api/me", tc.w)
		if code != http.StatusOK {
			t.Fatalf("%s: status %d", tc.w.name, code)
		}
		if me["tier"] != tc.wantTier {
			t.Errorf("%s: tier = %v, want %q", tc.w.name, me["tier"], tc.wantTier)
		}
		if me["scope"] != tc.wantScope {
			t.Errorf("%s: scope = %v, want %q", tc.w.name, me["scope"], tc.wantScope)
		}
		shapeLink, _ := me["shape"].(string)
		if (shapeLink != "") != tc.wantShape {
			t.Errorf("%s: shape = %q, wantPresent=%v", tc.w.name, shapeLink, tc.wantShape)
		}
	}
}

func TestPanelWishBoxByScopeAndMCPFlow(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}

	shapingJS, err := fs.ReadFile(assets, "public/shaping.js")
	if err != nil {
		t.Fatalf("read embedded shaping.js: %v", err)
	}

	scopedFS := fstest.MapFS{}
	for k, v := range fixture {
		scopedFS[k] = v
	}
	scopedFS["public/shaping.js"] = &fstest.MapFile{Data: shapingJS}
	scopedFS["shaped.json"] = &fstest.MapFile{Data: []byte(`{
	  "server": {"main": ".", "args": ["-addr", "$ADDR", "-data", "$DATA"]},
	  "scopes": {
	    "edit": {"circles": ["company"], "groups": ["support"]},
	    "shape": {"circles": ["contractor"], "groups": ["support"]}
	  }
	}`)}

	env := map[string]string{"SITE_SSO_SECRET": testSecret, "SITE_AI_KEY": "k"}
	s, err := newServerFrom(scopedFS, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}

	const mount = "/matrix/h/host/app/site"

	type mcpCall struct {
		Caller      string         `json:"caller"`
		Tool        string         `json:"tool"`
		Args        map[string]any `json:"args"`
		AuthHeader  string         `json:"auth_header"`
		CSRFHeader  string         `json:"csrf_header"`
		ProtoHeader string         `json:"proto_header"`
	}
	var (
		mu       sync.Mutex
		curWho   = nobody
		mcpCalls []mcpCall
		useSSE   bool
		candSeq  int
	)

	callers := map[string]who{
		"owner":     owner,
		"shape":     {"member with shape", "member", "contractor", "support", "", "c@example.com"},
		"edit":      {"member with edit", "member", "company", "support", "", "a@example.com"},
		"unscoped":  {"member without scope", "member", "company", "", "", "m@example.com"},
		"account":   stranger,
		"anonymous": nobody,
	}

	// Front simulates outpost mounted at /matrix/h/host/app/site: stamps
	// every request from the caller's cloudbox login and proxies <mount>/mcp
	// on the same origin to the site MCP server.
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_test/select" {
			mu.Lock()
			curWho = callers[r.URL.Query().Get("who")]
			useSSE = r.URL.Query().Get("sse") == "1"
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/_test/calls" {
			mu.Lock()
			snapshot := append([]mcpCall(nil), mcpCalls...)
			mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"calls": snapshot})
			return
		}
		if !strings.HasPrefix(r.URL.Path, mount) {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		activeWho := curWho
		sseMode := useSSE
		mu.Unlock()

		sub := strings.TrimPrefix(r.URL.Path, mount)
		if sub == "" {
			sub = "/"
		}
		if sub == "/mcp" {
			if r.Method != http.MethodPost {
				http.Error(w, "POST only", http.StatusMethodNotAllowed)
				return
			}
			var rpc struct {
				ID     any    `json:"id"`
				Method string `json:"method"`
				Params struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments"`
				} `json:"params"`
			}
			if err := json.NewDecoder(r.Body).Decode(&rpc); err != nil || rpc.Method != "tools/call" {
				http.Error(w, "bad jsonrpc", http.StatusBadRequest)
				return
			}
			mu.Lock()
			mcpCalls = append(mcpCalls, mcpCall{
				Caller:      activeWho.name,
				Tool:        rpc.Params.Name,
				Args:        rpc.Params.Arguments,
				AuthHeader:  r.Header.Get("Authorization"),
				CSRFHeader:  r.Header.Get("X-CSRF-Token"),
				ProtoHeader: r.Header.Get("MCP-Protocol-Version"),
			})
			var resultObj any
			isErr := false
			errText := ""
			switch rpc.Params.Name {
			case "shape_wish":
				candSeq++
				cid := "cand-" + strconv.Itoa(candSeq)
				resultObj = map[string]any{
					"app":       "site",
					"verb":      "wish",
					"principal": activeWho.user,
					"result":    map[string]any{"id": cid, "status": "ready", "rev": "abc123def456"},
				}
			case "shape_verify":
				cid, _ := rpc.Params.Arguments["id"].(string)
				resultObj = map[string]any{
					"app":       "site",
					"verb":      "verify",
					"principal": activeWho.user,
					"result":    map[string]any{"id": cid, "status": "verified", "verdict": map[string]any{"pass": true}},
				}
			case "shape_preview":
				cid, _ := rpc.Params.Arguments["id"].(string)
				resultObj = map[string]any{
					"app":       "site",
					"verb":      "preview",
					"principal": activeWho.user,
					"result":    map[string]any{"id": cid, "url": "http://127.0.0.1:7685/shape/site/preview/" + cid + "/"},
				}
			case "shape_approve":
				if activeWho.kind != "owner" {
					isErr = true
					errText = "refused: owner tier required"
				} else {
					cid, _ := rpc.Params.Arguments["id"].(string)
					resultObj = map[string]any{
						"app":       "site",
						"verb":      "approve",
						"principal": activeWho.user,
						"result":    map[string]any{"id": cid, "status": "approved", "rev": "abc123def456"},
					}
				}
			default:
				isErr = true
				errText = "unknown tool"
			}
			mu.Unlock()

			var mcpResult map[string]any
			if isErr {
				mcpResult = map[string]any{
					"isError": true,
					"content": []map[string]string{{"type": "text", "text": errText}},
				}
			} else {
				mcpResult = map[string]any{
					"structuredContent": resultObj,
				}
			}
			envelope := map[string]any{
				"jsonrpc": "2.0",
				"id":      rpc.ID,
				"result":  mcpResult,
			}
			raw, _ := json.Marshal(envelope)
			if sseMode {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "event: message\ndata: "+string(raw)+"\n\n")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(raw)
			return
		}

		// Forward page/API request to site-shaping with outpost's signed stamp.
		fwd := r.Clone(r.Context())
		fwd.URL.Path = sub
		if activeWho.kind != "" {
			stamp(fwd, activeWho.kind, activeWho.circle, activeWho.groups, activeWho.visitor, activeWho.user, time.Now())
		} else {
			fwd.Header.Set("X-Forwarded-Prefix", mount)
		}
		s.ServeHTTP(w, fwd)
	}))
	defer front.Close()

	scriptTmp := filepath.Join(t.TempDir(), "shaping.js")
	if err := os.WriteFile(scriptTmp, shapingJS, 0o644); err != nil {
		t.Fatal(err)
	}

	const domHarness = `
const fs = require("fs");
const vm = require("vm");
const assert = require("assert");

const scriptPath = process.argv[1];
const origin = process.argv[2];
const mount = process.argv[3];
const code = fs.readFileSync(scriptPath, "utf8");

function makeNode(tag, ns) {
  const attrs = {};
  const listeners = {};
  const classes = new Set();
  const styleMap = {};
  const node = {
    tagName: (tag || "").toUpperCase(),
    namespaceURI: ns || null,
    children: [],
    parentNode: null,
    value: "",
    disabled: false,
    isContentEditable: false,
    _text: "",
    style: {
      setProperty(k, v) { styleMap[k] = String(v); },
      removeProperty(k) { delete styleMap[k]; }
    },
    classList: {
      add(c) { classes.add(c); syncClass(); },
      remove(c) { classes.delete(c); syncClass(); },
      toggle(c, force) {
        const on = force === undefined ? !classes.has(c) : !!force;
        if (on) classes.add(c); else classes.delete(c);
        syncClass();
        return on;
      }
    },
    get textContent() {
      return this._text + this.children.map(c => c.textContent).join("");
    },
    set textContent(v) {
      this._text = String(v == null ? "" : v);
      this.children = [];
    },
    get firstChild() { return this.children[0] || null; },
    setAttribute(k, v) {
      attrs[k] = String(v);
      if (k === "class") {
        classes.clear();
        String(v).split(/\s+/).filter(Boolean).forEach(c => classes.add(c));
      }
    },
    getAttribute(k) { return Object.prototype.hasOwnProperty.call(attrs, k) ? attrs[k] : null; },
    hasAttribute(k) { return Object.prototype.hasOwnProperty.call(attrs, k); },
    removeAttribute(k) { delete attrs[k]; },
    appendChild(c) { c.parentNode = this; this.children.push(c); return c; },
    insertBefore(c, ref) {
      c.parentNode = this;
      const i = this.children.indexOf(ref);
      if (i >= 0) this.children.splice(i, 0, c);
      else this.children.push(c);
      return c;
    },
    remove() {
      if (!this.parentNode) return;
      const i = this.parentNode.children.indexOf(this);
      if (i >= 0) this.parentNode.children.splice(i, 1);
      this.parentNode = null;
    },
    addEventListener(ev, fn) { (listeners[ev] = listeners[ev] || []).push(fn); },
    click() { (listeners.click || []).forEach(fn => fn({ preventDefault() {} })); },
    querySelectorAll(sel) {
      const want = sel.split(",").map(s => s.trim());
      const out = [];
      (function walk(n) {
        for (const c of n.children) {
          for (const w of want) {
            if (w.startsWith(".") && (c.getAttribute("class") || "").split(/\s+/).includes(w.slice(1))) out.push(c);
            else if (c.tagName && c.tagName.toLowerCase() === w.toLowerCase()) out.push(c);
          }
          walk(c);
        }
      })(this);
      return out;
    },
    querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
  };
  function syncClass() { attrs["class"] = Array.from(classes).join(" "); }
  return node;
}

async function waitUntil(pred, msg) {
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    if (pred()) return;
    await new Promise(r => setTimeout(r, 15));
  }
  throw new Error("timeout: " + msg);
}

async function openFor(who, sse) {
  const sel = await fetch(origin + "/_test/select?who=" + encodeURIComponent(who) + "&sse=" + (sse ? "1" : "0"));
  assert.strictEqual(sel.status, 204);

  const docEl = makeNode("html");
  const body = makeNode("body");
  docEl.appendChild(body);
  const store = {};
  const document = {
    documentElement: docEl,
    body,
    cookie: "csrf_token=csrf-abc",
    hidden: false,
    activeElement: null,
    currentScript: { src: origin + mount + "/_shaping/shaping.js" },
    createElement: (tag) => makeNode(tag),
    createElementNS: (ns, tag) => makeNode(tag, ns),
    createTextNode: (t) => { const n = makeNode("#text"); n._text = String(t); return n; },
    getElementById(id) {
      let found = null;
      (function walk(n) {
        if (found) return;
        for (const c of n.children) {
          if (c.getAttribute("id") === id) { found = c; return; }
          walk(c);
        }
      })(body);
      return found;
    },
    querySelector: (s) => body.querySelector(s),
    querySelectorAll: (s) => body.querySelectorAll(s),
    addEventListener() {}
  };
  const sandbox = {
    document,
    location: { origin, pathname: mount + "/about", search: "", reload() {} },
    localStorage: {
      getItem: (k) => Object.prototype.hasOwnProperty.call(store, k) ? store[k] : null,
      setItem: (k, v) => { store[k] = String(v); },
      removeItem: (k) => { delete store[k]; }
    },
    URL,
    fetch,
    setInterval: () => 1,
    clearInterval: () => {},
    console
  };
  vm.runInNewContext(code, sandbox, { filename: scriptPath });
  await waitUntil(() => document.getElementById("ss-panel-root") !== null, "panel root mount for " + who);
  return document;
}

(async () => {
  // 1. Non-shape callers (edit-only member, unscoped member, account, anonymous) get NO wish box and NO console link.
  for (const who of ["edit", "unscoped", "account", "anonymous"]) {
    const doc = await openFor(who, false);
    assert.strictEqual(doc.querySelector(".ss-shape"), null, who + " must not see owner shape link");
    assert.strictEqual(doc.querySelector(".ss-shape-box"), null, who + " must not see shape wish box");
    assert.strictEqual(doc.querySelector(".ss-shape-approve"), null, who + " must not see approve");
  }

  // 2. Member with shape scope: gets wish box (no console shape link, no approve),
  //    posts wish/verify/preview to <mount>/mcp (including SSE framing), shows status + preview.
  const memberDoc = await openFor("shape", true);
  assert.strictEqual(memberDoc.querySelector(".ss-shape"), null, "member with shape must not see owner console link");
  const memberBox = memberDoc.querySelector(".ss-shape-box");
  assert.ok(memberBox, "member with shape must see wish box");
  const memberInput = memberBox.querySelector(".ss-wish-input");
  const memberSubmit = memberBox.querySelector(".ss-wish-submit");
  memberInput.value = "Add FAQ hours";
  memberSubmit.click();

  const memberStatus = memberBox.querySelector(".ss-shape-status");
  await waitUntil(() => /Verified \(cand-1\) · Preview ready/.test(memberStatus.textContent), "member verify/preview status: " + memberStatus.textContent);
  assert.ok(/awaiting owner approval/i.test(memberStatus.textContent), "member status should note owner approval: " + memberStatus.textContent);
  const memberPreview = memberBox.querySelector(".ss-shape-preview");
  assert.ok(memberPreview && /\/preview\/cand-1\/$/.test(memberPreview.getAttribute("href")), "member missing preview link");
  assert.strictEqual(memberBox.querySelector(".ss-shape-approve"), null, "member must NOT see approve button");

  // 3. Owner: sees console shape link, sees wish box, sees verify/preview status AND approve button, and approves.
  const ownerDoc = await openFor("owner", false);
  assert.ok(ownerDoc.querySelector(".ss-shape"), "owner must see console shape link");
  const ownerBox = ownerDoc.querySelector(".ss-shape-box");
  assert.ok(ownerBox, "owner must see wish box");
  ownerBox.querySelector(".ss-wish-input").value = "Publish holiday banner";
  ownerBox.querySelector(".ss-wish-submit").click();

  const ownerStatus = ownerBox.querySelector(".ss-shape-status");
  await waitUntil(() => /Verified \(cand-2\) · Preview ready/.test(ownerStatus.textContent), "owner verify/preview status: " + ownerStatus.textContent);
  const approveBtn = ownerBox.querySelector(".ss-shape-approve");
  assert.ok(approveBtn, "owner must additionally see approve button");
  approveBtn.click();
  await waitUntil(() => /Approved \(cand-2\)/.test(ownerStatus.textContent), "owner approve status: " + ownerStatus.textContent);
  assert.strictEqual(ownerBox.querySelector(".ss-shape-approve"), null, "approve button removed after approval");
  assert.strictEqual(ownerBox.querySelector(".ss-wish-input").value, "", "wish input cleared after approval");

  // 4. Inspect recorded MCP calls: only shape member + owner reached <mount>/mcp,
  //    no Authorization header/token/password was sent, CSRF header was echoed, page was "/about".
  const callsResp = await (await fetch(origin + "/_test/calls")).json();
  const calls = callsResp.calls;
  assert.deepStrictEqual(
    calls.map(c => c.caller + ":" + c.tool),
    [
      "member with shape:shape_wish",
      "member with shape:shape_verify",
      "member with shape:shape_preview",
      "owner:shape_wish",
      "owner:shape_verify",
      "owner:shape_preview",
      "owner:shape_approve"
    ]
  );
  for (const c of calls) {
    assert.strictEqual(c.auth_header, "", "must not send Authorization token/password: " + JSON.stringify(c));
    assert.strictEqual(c.csrf_header, "csrf-abc", "must echo X-CSRF-Token: " + JSON.stringify(c));
    assert.strictEqual(c.proto_header, "2025-06-18", "unexpected MCP-Protocol-Version");
  }
  assert.strictEqual(calls[0].args.request, "Add FAQ hours");
  assert.strictEqual(calls[0].args.page, "/about");
  assert.strictEqual(calls[3].args.request, "Publish holiday banner");
  assert.strictEqual(calls[3].args.page, "/about");
})().catch(err => { console.error(err); process.exit(1); });
`

	cmd := exec.Command(nodeBin, "-e", domHarness, "--", scriptTmp, front.URL, mount)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("DOM wish box test failed: %v\n%s", err, out)
	}
}
