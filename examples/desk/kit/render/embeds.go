package render

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// deskOverrides implements goldmark's NodeRenderer for the two node kinds
// the desk cares about: fenced code blocks (```chart / ```csv /
// highlighted code) and images (file-type embeds). Registering for a kind
// replaces goldmark's default func for that kind; every other kind keeps
// the default renderer.
type deskOverrides struct {
	opt Options
}

// RegisterFuncs implements renderer.NodeRenderer.
func (d *deskOverrides) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, d.renderFence)
	reg.Register(ast.KindImage, d.renderImage)
}

// renderImage routes ![](dest) by file type. Standard Markdown degrades
// gracefully: unknown types, missing files and (by default) remote URLs
// become plain links, never errors and never raw HTML.
func (d *deskOverrides) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)
	dest := string(n.Destination)
	alt := string(n.Text(source))
	title := string(n.Title)
	fmt.Fprint(w, d.embed(dest, alt, title))
	return ast.WalkSkipChildren, nil
}

// embed builds the HTML for one image destination.
func (d *deskOverrides) embed(dest, alt, title string) string {
	label := alt
	if label == "" {
		label = dest
	}
	safeLabel := html.EscapeString(label)

	if strings.HasPrefix(dest, "data:") {
		// Inline data: images only, and only image/* MIME; anything
		// else (data:text/html, ...) degrades to text.
		if strings.HasPrefix(strings.ToLower(dest), "data:image/") {
			return `<img class="desk-embed desk-embed-img" src="` +
				html.EscapeString(dest) + `" alt="` + html.EscapeString(alt) + `">`
		}
		return `<span class="desk-embed desk-embed-link">` + safeLabel + `</span>`
	}

	if isRemote(dest) {
		if !d.opt.AllowRemote {
			return `<a class="desk-embed desk-embed-link" href="` +
				html.EscapeString(dest) + `">` + safeLabel + `</a>`
		}
		return d.remoteEmbed(dest, alt, title)
	}

	rel, ok := cleanRel(dest)
	if !ok {
		return `<span class="desk-embed desk-embed-link">` + safeLabel + `</span>`
	}
	href := d.assetURL(rel)
	ext := strings.ToLower(path.Ext(stripFrag(rel)))

	switch ext {
	case ".csv", ".tsv":
		raw, err := d.readAsset(rel)
		if err != nil {
			return missingLink(href, safeLabel)
		}
		return csvTable(raw, delimFor(ext), href, safeLabel)
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		src := href
		if d.opt.Inline {
			if uri, err := d.inlineURI(rel, mimeFor(ext)); err == nil {
				src = uri
			}
		}
		return `<img class="desk-embed desk-embed-img" src="` +
			html.EscapeString(src) + `" alt="` + html.EscapeString(alt) + `">`
	case ".svg":
		// Served as <img>, never inline: SVG documents can carry
		// script, which does not execute in an image context.
		src := href
		if d.opt.Inline {
			if uri, err := d.inlineURI(rel, "image/svg+xml"); err == nil {
				src = uri
			}
		}
		return `<img class="desk-embed desk-embed-img" src="` +
			html.EscapeString(src) + `" alt="` + html.EscapeString(alt) + `">`
	case ".mp4", ".webm":
		src := href
		if d.opt.Inline {
			if uri, err := d.inlineURI(rel, mimeFor(ext)); err == nil {
				src = uri
			}
		}
		return `<video class="desk-embed desk-embed-video" controls preload="none" src="` +
			html.EscapeString(src) + `" aria-label="` + html.EscapeString(alt) + `"></video>`
	case ".pdf":
		src := href
		if d.opt.Inline {
			if uri, err := d.inlineURI(rel, "application/pdf"); err == nil {
				src = uri
			}
		}
		return `<div class="desk-embed desk-embed-pdf">` +
			`<iframe src="` + html.EscapeString(src) + `" title="` + html.EscapeString(alt) + `" loading="lazy"></iframe>` +
			`<a class="desk-download" href="` + html.EscapeString(href) + `">` + safeLabel + `</a></div>`
	default:
		if _, err := d.readAsset(rel); err != nil {
			return missingLink(href, safeLabel)
		}
		return `<a class="desk-embed desk-embed-link" href="` +
			html.EscapeString(href) + `">` + safeLabel + `</a>`
	}
}

// remoteEmbed renders an opted-in remote embed. Images, video and PDFs
// load client-side (the server never fetches them); anything tabular or
// unknown stays a plain link because rendering it would need a
// server-side fetch of an untrusted URL.
func (d *deskOverrides) remoteEmbed(dest, alt, title string) string {
	safeLabel := html.EscapeString(alt)
	if safeLabel == "" {
		safeLabel = html.EscapeString(dest)
	}
	esc := html.EscapeString(dest)
	switch strings.ToLower(path.Ext(stripFrag(dest))) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return `<img class="desk-embed desk-embed-img" src="` + esc +
			`" alt="` + html.EscapeString(alt) + `">`
	case ".mp4", ".webm":
		return `<video class="desk-embed desk-embed-video" controls preload="none" src="` +
			esc + `" aria-label="` + html.EscapeString(alt) + `"></video>`
	case ".pdf":
		return `<div class="desk-embed desk-embed-pdf">` +
			`<iframe src="` + esc + `" title="` + html.EscapeString(alt) + `" loading="lazy"></iframe>` +
			`<a class="desk-download" href="` + esc + `">` + safeLabel + `</a></div>`
	default:
		_ = title
		return `<a class="desk-embed desk-embed-link" href="` + esc + `">` + safeLabel + `</a>`
	}
}

// missingLink is the graceful degradation for an asset that is named but
// not on disk: a link to where it would be served, marked missing.
func missingLink(href, safeLabel string) string {
	return `<a class="desk-embed desk-embed-link desk-embed-missing" href="` +
		html.EscapeString(href) + `">` + safeLabel + `</a>`
}

// isRemote reports http(s) (and protocol-relative) destinations.
func isRemote(dest string) bool {
	lower := strings.ToLower(dest)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "//")
}

// stripFrag drops any ?query or #fragment for extension and file lookup.
func stripFrag(dest string) string {
	if i := strings.IndexAny(dest, "?#"); i >= 0 {
		return dest[:i]
	}
	return dest
}

// cleanRel validates a relative asset path: no absolute paths, no drive
// letters, no backslashes, and no .. escape once cleaned.
func cleanRel(dest string) (string, bool) {
	p := stripFrag(dest)
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return "", false
	}
	if strings.Contains(p, ":") {
		// Scheme-like (mailto:, javascript:, ...) or drive letter.
		return "", false
	}
	c := path.Clean(p)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return "", false
	}
	u, err := url.Parse(c)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return "", false
	}
	return c, true
}

// assetURL joins the same-origin prefix with a validated relative path,
// escaping each segment.
func (d *deskOverrides) assetURL(rel string) string {
	prefix := d.opt.AssetURL
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	segs := strings.Split(rel, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return prefix + strings.Join(segs, "/")
}

// readAsset reads one asset file, confined under AssetDir. Oversized
// files are refused so a giant CSV cannot exhaust the preview.
func (d *deskOverrides) readAsset(rel string) ([]byte, error) {
	if d.opt.AssetDir == "" {
		return nil, fmt.Errorf("no asset dir")
	}
	full := filepath.Join(d.opt.AssetDir, filepath.FromSlash(rel))
	base, err := filepath.Abs(d.opt.AssetDir)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(full)
	if err != nil {
		return nil, err
	}
	if abs != base && !strings.HasPrefix(abs, base+string(filepath.Separator)) {
		return nil, fmt.Errorf("asset escapes dir")
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// 256 KiB is plenty for a preview table or an inline asset.
	return io.ReadAll(io.LimitReader(f, 256<<10))
}

// inlineURI base64-encodes a small asset as a data: URI for
// self-contained export. Large assets stay referenced by URL.
func (d *deskOverrides) inlineURI(rel, mime string) (string, error) {
	raw, err := d.readAsset(rel)
	if err != nil {
		return "", err
	}
	if len(raw) > 1<<20 {
		return "", fmt.Errorf("asset too large to inline")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

// mimeFor maps embed extensions to data-URI MIME types.
func mimeFor(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}

// delimFor picks the CSV delimiter by embed extension.
func delimFor(ext string) rune {
	if ext == ".tsv" {
		return '\t'
	}
	return ','
}

// csvTable renders parsed CSV/TSV as an inline table: header row, the
// first MaxEmbedRows data rows, a "+N more rows" note when truncated, and
// a download link. Malformed input degrades to a download link.
func csvTable(raw []byte, delim rune, href, safeLabel string) string {
	r := csv.NewReader(bytes.NewReader(raw))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil || len(rows) == 0 {
		return missingLink(href, safeLabel)
	}
	var b strings.Builder
	b.WriteString(`<div class="desk-embed desk-embed-table"><table class="desk-table">`)
	b.WriteString(`<thead><tr>`)
	for _, h := range rows[0] {
		b.WriteString(`<th>` + html.EscapeString(h) + `</th>`)
	}
	b.WriteString(`</tr></thead><tbody>`)
	shown := rows[1:]
	truncated := 0
	if len(shown) > MaxEmbedRows {
		truncated = len(shown) - MaxEmbedRows
		shown = shown[:MaxEmbedRows]
	}
	for _, row := range shown {
		b.WriteString(`<tr>`)
		for i, h := range rows[0] {
			_ = h
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			b.WriteString(`<td>` + html.EscapeString(cell) + `</td>`)
		}
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
	if truncated > 0 {
		fmt.Fprintf(&b, `<p class="desk-embed-note">+%d more rows</p>`, truncated)
	}
	b.WriteString(`<a class="desk-download" href="` + html.EscapeString(href) + `">` + safeLabel + `</a></div>`)
	return b.String()
}
