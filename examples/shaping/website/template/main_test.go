package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	name                        string
	kind, circle, visitor, user string
}

var (
	owner          = who{"owner", "owner", "", "", "owner@example.com"}
	recruiter      = who{"recruiter", "member", "recruiter", "", "rec@example.com"}
	friend         = who{"friend", "member", "friend", "", "pal@example.com"}
	stranger       = who{"account", "account", "", "0123456789abcdef0123456789abcdef", ""}
	strangerLegacy = who{"signed-in alias", "signed-in", "", "0123456789abcdef0123456789abcdef", ""}
	nobody         = who{"anonymous", "", "", "", ""}
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
		stamp(req, w.kind, w.circle, "", w.visitor, w.user, time.Now())
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
		{"/_shaping/shaping.css", "ss-theme-dark"},
		{"/_shaping/shaping.css", "ss-motion-reduce"},
	} {
		req := httptest.NewRequest("GET", tc.path, nil)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: status=%d missing %q", tc.path, rec.Code, tc.want)
		}
	}
}
