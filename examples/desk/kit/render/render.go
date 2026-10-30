// Package render is the ONE Markdown renderer shared by every desk tool
// that shows human-edited Markdown (notepad preview, viewer, HTML export).
//
// Tools must not roll their own Markdown-to-HTML path: notepad preview,
// the read-only viewer and HTML export all call Markdown with their item's
// Options so the three views cannot drift apart.
//
// Pipeline: goldmark (GFM + footnotes + definition lists + auto heading
// ids; raw HTML pass-through DISABLED) with desk overrides for fenced
// ```chart / ```csv blocks and for file-type image embeds, then a
// bluemonday allowlist pass, then semantic-class post-processing. The
// package emits SEMANTIC CLASSES ONLY (desk-content, desk-table, desk-code,
// desk-toc, desk-embed-*) and no CSS: styling belongs to kit.css, owned by
// S537.D1.
//
// Dependencies (license-checked 2026-10-30, all permissive pure Go):
//
//	goldmark v1.8.2 (MIT) — Markdown parsing, GFM extensions.
//	bluemonday (BSD-3-Clause) — allowlist sanitizer over goldmark output.
//	chroma/v2 (MIT) — server-side code highlighting, class-based output
//	  (no inline styles, so the strict view CSP keeps applying).
package render

import (
	"bytes"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// Options tunes one Markdown rendering for one item.
type Options struct {
	// AssetDir is the item asset dir
	// ($BASHY_HOME/desk/<tool>/items/<id>/assets). Relative embeds
	// resolve under it only; anything escaping it degrades to a link.
	AssetDir string
	// AssetURL is the same-origin URL prefix under which AssetDir is
	// served (e.g. "/assets/<id>/"). Embed src/href attributes are
	// built from it; no other origin is ever emitted for local files.
	AssetURL string
	// AllowRemote gates http(s) embeds. Default false: remote embeds
	// render as plain links and are never fetched server-side.
	AllowRemote bool
	// Inline is the self-contained export switch: local image, video
	// and PDF assets are inlined as data: URIs. Tables are already
	// inline by construction; their download links keep AssetURL hrefs.
	Inline bool
}

// MaxEmbedRows caps how many data rows a csv/tsv embed or fence shows;
// the remainder is reported as "+N more rows" beside the download link.
const MaxEmbedRows = 25

// newConverter builds the per-item converter: goldmark with GFM (tables,
// task lists, strikethrough, autolinks), footnotes, definition lists and
// auto heading ids, plus the desk fence/image overrides bound to opt. The
// converter is built per call because the overrides carry per-item asset
// options; construction is registration-only (no I/O) and cheap. Raw HTML
// stays disabled, so author-supplied HTML never passes through; the
// bluemonday policy in sanitize.go is the second layer (javascript: URLs,
// event attributes).
func newConverter(opt Options) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.DefinitionList,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			// Priority must be BELOW the default HTML renderer's 1000:
			// goldmark registers same-kind funcs highest-first, so the
			// lowest priority value wins the kind.
			renderer.WithNodeRenderers(
				util.Prioritized(&deskOverrides{opt: opt}, 500),
			),
		),
	)
}

// Markdown renders src to a sanitized HTML fragment wrapped in
// <div class="desk-content"> — the same class kit.js marks and kit.css
// styles (D1), so server and client rendering share one look. It never
// passes raw HTML through and never fetches remote URLs; failures to read
// a local asset degrade that embed to a plain link instead of failing the
// whole render.
func Markdown(src []byte, opt Options) (html []byte, err error) {
	if len(src) > 1<<20 {
		src = src[:1<<20]
	}
	var buf bytes.Buffer
	if err := newConverter(opt).Convert(src, &buf); err != nil {
		return nil, err
	}
	out := sanitize(buf.Bytes())
	out = postProcess(out)
	return []byte(`<div class="desk-content">` + string(out) + `</div>`), nil
}

// Page wraps a Markdown fragment in a full view page: strict
// content-security-policy, kit CSS/JS only (sibling kit.css/kit.js
// routes served by the tool), no inline scripts or styles, no forms.
// connect-src is absent so the rendered view cannot issue API calls,
// mutating or otherwise.
func Page(title string, body []byte) []byte {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<meta http-equiv="Content-Security-Policy" content="` + csp + `">`)
	b.WriteString(`<title>` + html.EscapeString(title) + `</title>`)
	b.WriteString(`<link rel="stylesheet" href="kit.css">`)
	b.WriteString(`</head><body>`)
	b.Write(body)
	b.WriteString(`<script src="kit.js" defer></script></body></html>`)
	return []byte(b.String())
}

// csp is the strict view policy: self-only subresources, same-origin
// frames (the built-in PDF viewer), data: images/media (Inline export),
// and no connect/form/frame-ancestor surface at all.
const csp = `default-src 'none'; base-uri 'none'; form-action 'none'; ` +
	`frame-ancestors 'none'; img-src 'self' data:; media-src 'self' data:; ` +
	`frame-src 'self'; font-src 'self'; style-src 'self'; script-src 'self'`

var (
	tableRe = regexp.MustCompile(`<table>`)
	preRe   = regexp.MustCompile(`<pre><code`)
	// A paragraph holding only [toc] / [[toc]] (any case) becomes the TOC.
	tocParaRe = regexp.MustCompile(`(?i)<p>\[?\[toc\]\]?</p>`)
	headingRe = regexp.MustCompile(`<h([1-6]) id="([^"]+)">(.*?)</h[1-7]>`)
	tagRe     = regexp.MustCompile(`<[^>]+>`)
)

// postProcess applies the semantic-class layer and the TOC expansion on
// sanitized HTML: GFM tables gain desk-table, code blocks gain desk-code,
// and a [toc] paragraph becomes a nav over the rendered headings.
func postProcess(in []byte) []byte {
	s := string(in)
	s = tableRe.ReplaceAllString(s, `<table class="desk-table">`)
	s = preRe.ReplaceAllString(s, `<pre class="desk-code"><code`)
	// The sanitizer lowercases attribute names; SVG viewBox is
	// case-sensitive, so restore it on our own chart output.
	s = strings.ReplaceAll(s, "viewbox=", "viewBox=")
	if tocParaRe.MatchString(s) {
		s = tocParaRe.ReplaceAllString(s, buildTOC(s))
	}
	return []byte(s)
}

// buildTOC lists every rendered heading (which already carry goldmark
// auto ids) as a nav. It runs on sanitized HTML so heading text cannot
// smuggle markup into the links.
func buildTOC(s string) string {
	matches := headingRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return `<nav class="desk-toc"></nav>`
	}
	var b strings.Builder
	b.WriteString(`<nav class="desk-toc" aria-label="Contents"><ul>`)
	for _, m := range matches {
		text := strings.TrimSpace(tagRe.ReplaceAllString(m[3], ""))
		if text == "" {
			continue
		}
		b.WriteString(`<li class="desk-toc-l` + m[1] + `"><a href="#` + m[2] + `">` +
			html.EscapeString(text) + `</a></li>`)
	}
	b.WriteString(`</ul></nav>`)
	return b.String()
}
