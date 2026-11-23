package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// fixture is an isolated site asset tree so unit tests never depend on an
// owner's edited site.json or content/*.md.
var fixture = fstest.MapFS{
	"site.json": {Data: []byte(`{
  "title": "Fixture Org",
  "origin": "https://fixture.example",
  "owner": "fixture",
  "nav": [
    {"label": "Home", "href": "/"},
    {"label": "About", "href": "/about"},
    {"label": "Source", "href": "https://example.org/repo"}
  ]
}`)},
	"content/home.md": {Data: []byte(`---
title: Fixture Org
path: /
---

## What It Is {#what}

Fixture Org turns plain Markdown into deterministic representations. Read [About](/about) or jump to [Team](/about#team).

## Quick Start {#start}

Install in one line with ` + "`curl -fsSL https://fixture.example/install.sh`" + ` (inline code ` + "`[ignored](/not-a-link)`" + ` is not a link):

` + "```sh\n# [code comment](/also-not-a-link)\necho ready\n```" + `

## Shape {#shape}

Open [Shape this page](/?shape=open) to customize presentation.
`)},
	"content/about.md": {Data: []byte(`---
title: About Fixture Org
path: /about
---

## Overview {#about}

Learn how [Fixture Org](/) keeps visitors and agents on one source.

## Engineering Team {#team}

- Deterministic rendering across HTML, Markdown, and JSON.
- Explicit section identifiers, never heading slugs.

## Contact {#contact}

Email [team@fixture.example](mailto:team@fixture.example) or visit [Quick Start](/#start).
`)},
}

func writeMapFSToDir(t *testing.T, m fstest.MapFS) string {
	t.Helper()
	dir := t.TempDir()
	for rel, file := range m {
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, file.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func cloneMapFS(m fstest.MapFS) fstest.MapFS {
	out := fstest.MapFS{}
	for k, v := range m {
		cp := make([]byte, len(v.Data))
		copy(cp, v.Data)
		out[k] = &fstest.MapFile{Data: cp}
	}
	return out
}

func TestEmbeddedAssetsPassStructuralInvariants(t *testing.T) {
	site, err := LoadSite(assets)
	if err != nil {
		t.Fatalf("LoadSite(assets): %v", err)
	}
	if len(site.Pages) == 0 {
		t.Fatal("embedded site has no pages")
	}
	if err := ValidateSite(site); err != nil {
		t.Fatalf("ValidateSite(assets): %v", err)
	}
}

func TestSectionIDsMustBeDeclaredNeverDerived(t *testing.T) {
	bad := cloneMapFS(fixture)
	bad["content/about.md"] = &fstest.MapFile{Data: []byte("---\ntitle: About\npath: /about\n---\n\n## Team Without ID\n\nCopy.\n")}
	_, err := LoadSite(bad)
	if err == nil || !strings.Contains(err.Error(), "lacks a declared {#id}") {
		t.Fatalf("expected missing {#id} refusal, got: %v", err)
	}

	// Heading text can change freely without moving a declared id; and two
	// completely different words keep the exact id declared in {#...}.
	site, err := LoadSite(fixture)
	if err != nil {
		t.Fatal(err)
	}
	about := site.ByPath["/about"]
	if about.Sections[1].Heading != "Engineering Team" || about.Sections[1].ID != "team" {
		t.Fatalf("got heading=%q id=%q, want Engineering Team / team", about.Sections[1].Heading, about.Sections[1].ID)
	}
}

func TestRevisionSensitivityAndRenderingInvariance(t *testing.T) {
	site1, err := LoadSite(fixture)
	if err != nil {
		t.Fatal(err)
	}
	about1 := site1.ByPath["/about"]
	if len(about1.Revision) != 12 {
		t.Fatalf("revision %q length = %d, want 12 hex chars", about1.Revision, len(about1.Revision))
	}

	// Non-semantic whitespace / CRLF normalization and origin change (a
	// rendering/deployment parameter, not copy) must NOT change revision.
	crlf := cloneMapFS(fixture)
	crlf["content/about.md"] = &fstest.MapFile{
		Data: []byte(strings.ReplaceAll(string(fixture["content/about.md"].Data), "\n", "\r\n") + "\r\n\r\n"),
	}
	siteCRLF, err := LoadSite(crlf)
	if err != nil {
		t.Fatal(err)
	}
	siteCRLF.SetOrigin("https://staging.fixture.example")
	if got := siteCRLF.ByPath["/about"].Revision; got != about1.Revision {
		t.Fatalf("CRLF/origin rendering change moved revision: %s != %s", got, about1.Revision)
	}

	// A copy change in #team MUST change the page revision and #team's sha256,
	// while leaving #about's section sha256 unchanged.
	edited := cloneMapFS(fixture)
	edited["content/about.md"] = &fstest.MapFile{
		Data: []byte(strings.Replace(string(fixture["content/about.md"].Data), "Explicit section identifiers", "Updated copy in team section", 1)),
	}
	siteEdited, err := LoadSite(edited)
	if err != nil {
		t.Fatal(err)
	}
	aboutEdited := siteEdited.ByPath["/about"]
	if aboutEdited.Revision == about1.Revision {
		t.Fatalf("copy edit did not change page revision (%s)", about1.Revision)
	}
	if aboutEdited.Sections[0].SHA256 != about1.Sections[0].SHA256 {
		t.Error("unchanged section #about changed sha256")
	}
	if aboutEdited.Sections[1].SHA256 == about1.Sections[1].SHA256 {
		t.Error("edited section #team kept old sha256")
	}
}

func TestThreeRepresentationsListSameLinksAndSectionIDs(t *testing.T) {
	site, err := LoadSite(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range site.Pages {
		frag := p.RenderHTMLFragment()
		doc := site.RenderHTMLDocument(p)
		md := p.RenderMarkdown()
		rawJSON, err := p.RenderJSON()
		if err != nil {
			t.Fatal(err)
		}

		wantIDs := ExtractHTMLSectionIDs(frag)
		if len(wantIDs) == 0 {
			t.Fatalf("%s: no section ids", p.Path)
		}
		if got := ExtractHTMLSectionIDs(doc); !slices.Equal(got, wantIDs) {
			t.Errorf("%s html-doc ids %v != %v", p.Path, got, wantIDs)
		}
		if got := ExtractMarkdownSectionIDs(md); !slices.Equal(got, wantIDs) {
			t.Errorf("%s markdown ids %v != %v", p.Path, got, wantIDs)
		}
		if got, err := ExtractJSONSectionIDs(rawJSON); err != nil || !slices.Equal(got, wantIDs) {
			t.Errorf("%s json ids %v (err=%v) != %v", p.Path, got, err, wantIDs)
		}

		wantLinks := ExtractHTMLLinks(frag)
		if got := ExtractHTMLLinks(doc); !slices.Equal(got, wantLinks) {
			t.Errorf("%s html-doc links %v != %v", p.Path, got, wantLinks)
		}
		if got := ExtractMarkdownLinks(md); !slices.Equal(got, wantLinks) {
			t.Errorf("%s markdown links %v != %v", p.Path, got, wantLinks)
		}
		if got, err := ExtractJSONLinks(rawJSON); err != nil || !slices.Equal(got, wantLinks) {
			t.Errorf("%s json links %v (err=%v) != %v", p.Path, got, err, wantLinks)
		}
	}
}

func TestServerNegotiationTwinsHeadersAndIndexes(t *testing.T) {
	srv, err := newServerFrom(fixture, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	aboutRev := srv.site.ByPath["/about"].Revision
	homeRev := srv.site.ByPath["/"].Revision
	wantETag := `"` + aboutRev + `"`

	request := func(method, target, accept, inm string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	// 1. Default, browser Accept, and */* serve HTML with canonical, alternates, and meta revision.
	for _, accept := range []string{
		"",
		"*/*",
		"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	} {
		rec := request("GET", "/about", accept, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /about (Accept=%q): status %d", accept, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("Accept=%q Content-Type = %q, want text/html", accept, ct)
		}
		if rec.Header().Get("Vary") != "Accept" {
			t.Errorf("Vary = %q, want Accept", rec.Header().Get("Vary"))
		}
		if rec.Header().Get("ETag") != wantETag {
			t.Errorf("ETag = %q, want %q", rec.Header().Get("ETag"), wantETag)
		}
		linkHdr := rec.Header().Get("Link")
		for _, want := range []string{
			`</about.md>; rel="alternate"; type="text/markdown"`,
			`</about.json>; rel="alternate"; type="application/json"`,
		} {
			if !strings.Contains(linkHdr, want) {
				t.Errorf("Link header %q missing %s", linkHdr, want)
			}
		}
		body := rec.Body.String()
		for _, want := range []string{
			`<meta name="revision" content="` + aboutRev + `">`,
			`<link rel="canonical" href="https://fixture.example/about">`,
			`<link rel="alternate" type="text/markdown" href="/about.md">`,
			`<link rel="alternate" type="application/json" href="/about.json">`,
			`<section id="team">`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("HTML body missing %s", want)
			}
		}
	}

	// 2. Accept: text/markdown returns Markdown; immediately following HTML request still returns HTML (no bleed).
	mdRec := request("GET", "/about", "text/markdown, text/html;q=0.8", "")
	if mdRec.Code != http.StatusOK || !strings.HasPrefix(mdRec.Header().Get("Content-Type"), "text/markdown") {
		t.Fatalf("Markdown negotiation failed: %d %s", mdRec.Code, mdRec.Header().Get("Content-Type"))
	}
	if mdRec.Header().Get("ETag") != wantETag || mdRec.Header().Get("Vary") != "Accept" {
		t.Errorf("Markdown headers: ETag=%q Vary=%q", mdRec.Header().Get("ETag"), mdRec.Header().Get("Vary"))
	}
	for _, want := range []string{
		"path: /about\n",
		"revision: " + aboutRev + "\n",
		"markdown: /about.md\n",
		"json: /about.json\n",
		"[Home](/) · [About](/about) · [Source](https://example.org/repo)",
		"## Engineering Team {#team}",
	} {
		if !strings.Contains(mdRec.Body.String(), want) {
			t.Errorf("Markdown body missing %q:\n%s", want, mdRec.Body.String())
		}
	}

	afterHTML := request("GET", "/about", "", "")
	if !strings.HasPrefix(afterHTML.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("variant bleed after Markdown request: got %s", afterHTML.Header().Get("Content-Type"))
	}

	// 3. Accept: application/json returns JSON with exact schema.
	jsonRec := request("GET", "/about", "application/json", "")
	if jsonRec.Code != http.StatusOK || !strings.HasPrefix(jsonRec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("JSON negotiation failed: %d %s", jsonRec.Code, jsonRec.Header().Get("Content-Type"))
	}
	if jsonRec.Header().Get("ETag") != wantETag || jsonRec.Header().Get("Vary") != "Accept" {
		t.Errorf("JSON headers: ETag=%q Vary=%q", jsonRec.Header().Get("ETag"), jsonRec.Header().Get("Vary"))
	}
	var decoded Page
	if err := json.Unmarshal(jsonRec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if decoded.Path != "/about" || decoded.Revision != aboutRev || len(decoded.Sections) != 3 || decoded.Sections[1].ID != "team" || len(decoded.Sections[1].SHA256) != 64 {
		t.Fatalf("unexpected JSON payload: %+v", decoded)
	}

	// 4. Direct twins: /index.md, /index.json, /about.md, /about.json.
	for _, tc := range []struct {
		path, wantCT, wantETag string
	}{
		{"/index.md", "text/markdown", `"` + homeRev + `"`},
		{"/index.json", "application/json", `"` + homeRev + `"`},
		{"/about.md", "text/markdown", wantETag},
		{"/about.json", "application/json", wantETag},
	} {
		rec := request("GET", tc.path, "text/html", "")
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), tc.wantCT) {
			t.Errorf("twin %s: status=%d ct=%q", tc.path, rec.Code, rec.Header().Get("Content-Type"))
		}
		if rec.Header().Get("ETag") != tc.wantETag || rec.Header().Get("Vary") != "Accept" {
			t.Errorf("twin %s headers: ETag=%q Vary=%q", tc.path, rec.Header().Get("ETag"), rec.Header().Get("Vary"))
		}
	}

	// 5. Strong ETag If-None-Match returns 304; weak ETag does not.
	if rec := request("GET", "/about", "", wantETag); rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Errorf("strong If-None-Match: status=%d len=%d", rec.Code, rec.Body.Len())
	}
	if rec := request("GET", "/about", "", "W/"+wantETag); rec.Code != http.StatusOK {
		t.Errorf("weak If-None-Match matched strong ETag: status=%d", rec.Code)
	}

	// 6. /robots.txt and /llms.txt.
	robots := request("GET", "/robots.txt", "", "")
	if robots.Code != http.StatusOK || !strings.Contains(robots.Body.String(), "User-agent: *\nAllow: /\n") {
		t.Errorf("/robots.txt: %d %q", robots.Code, robots.Body.String())
	}
	llms := request("GET", "/llms.txt", "", "")
	if llms.Code != http.StatusOK {
		t.Fatalf("/llms.txt: %d", llms.Code)
	}
	for _, want := range []string{
		"# Fixture Org",
		"https://fixture.example/",
		"https://fixture.example/index.md",
		"https://fixture.example/about",
		"https://fixture.example/about.md",
	} {
		if !strings.Contains(llms.Body.String(), want) {
			t.Errorf("/llms.txt missing %q:\n%s", want, llms.Body.String())
		}
	}

	// 7. Read-only enforcement and 404.
	if rec := request("POST", "/about", "", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /about = %d, want 405", rec.Code)
	}
	if rec := request("GET", "/missing", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET /missing = %d, want 404", rec.Code)
	}
}

func TestChecksDeterminismUniqueIDsLinksAndStaleBase(t *testing.T) {
	goodDir := writeMapFSToDir(t, fixture)
	outcomes, err := RunChecks(goodDir, goodDir, "all", "", "")
	if err != nil {
		t.Fatalf("RunChecks(good): %v (%+v)", err, outcomes)
	}
	if len(outcomes) != 4 {
		t.Fatalf("expected 4 checks, got %d", len(outcomes))
	}

	// Render determinism guarantees byte-identical outputs across invocations.
	s1, _ := LoadSiteDir(goodDir)
	s2, _ := LoadSiteDir(goodDir)
	j1, _ := s1.ByPath["/"].RenderJSON()
	j2, _ := s2.ByPath["/"].RenderJSON()
	if !bytes.Equal(j1, j2) {
		t.Fatal("JSON output not deterministic")
	}

	// Duplicate section id fails unique-ids.
	dupSec := cloneMapFS(fixture)
	dupSec["content/about.md"] = &fstest.MapFile{
		Data: []byte("---\ntitle: About\npath: /about\n---\n\n## One {#team}\n\nA.\n\n## Two {#team}\n\nB.\n"),
	}
	if err := CheckUniqueIDs(writeMapFSToDir(t, dupSec)); err == nil || !strings.Contains(err.Error(), "duplicate section id") {
		t.Errorf("expected duplicate section id error, got %v", err)
	}

	// Duplicate page path fails unique-ids.
	dupPath := cloneMapFS(fixture)
	dupPath["content/extra.md"] = &fstest.MapFile{
		Data: []byte("---\ntitle: Extra\npath: /about\n---\n\n## Extra {#extra}\n\nCopy.\n"),
	}
	if err := CheckUniqueIDs(writeMapFSToDir(t, dupPath)); err == nil || !strings.Contains(err.Error(), "duplicate page path") {
		t.Errorf("expected duplicate page path error, got %v", err)
	}

	// Broken internal page link and broken #fragment fail links-resolve.
	badPageLink := cloneMapFS(fixture)
	badPageLink["content/about.md"] = &fstest.MapFile{
		Data: []byte("---\ntitle: About\npath: /about\n---\n\n## Team {#team}\n\nSee [nowhere](/missing-page).\n"),
	}
	if err := CheckLinksResolve(writeMapFSToDir(t, badPageLink)); err == nil || !strings.Contains(err.Error(), "broken internal link") {
		t.Errorf("expected broken internal link error, got %v", err)
	}

	badFragLink := cloneMapFS(fixture)
	badFragLink["content/about.md"] = &fstest.MapFile{
		Data: []byte("---\ntitle: About\npath: /about\n---\n\n## Team {#team}\n\nSee [missing anchor](/#nonexistent).\n"),
	}
	if err := CheckLinksResolve(writeMapFSToDir(t, badFragLink)); err == nil || !strings.Contains(err.Error(), "missing section #nonexistent") {
		t.Errorf("expected missing section anchor error, got %v", err)
	}

	// Stale-base compare-and-set: matching base revision passes; stale revision fails with "stale, re-read".
	aboutBaseRev := s1.ByPath["/about"].Revision
	if err := CheckStaleBase(goodDir, "/about", "section=team revision="+aboutBaseRev); err != nil {
		t.Errorf("matching base revision failed: %v", err)
	}
	if err := CheckStaleBase(goodDir, "", "section=team revision="+aboutBaseRev); err != nil {
		t.Errorf("matching base revision (inferred page) failed: %v", err)
	}
	if err := CheckStaleBase(goodDir, "/about", "section=team revision=000000000000"); err == nil || !strings.Contains(err.Error(), "stale, re-read") {
		t.Errorf("stale revision: got %v, want 'stale, re-read'", err)
	}
	if err := CheckStaleBase(goodDir, "/about", "section=nonexistent revision="+aboutBaseRev); err == nil || !strings.Contains(err.Error(), "stale, re-read") {
		t.Errorf("missing section in wish: got %v, want 'stale, re-read'", err)
	}
}
