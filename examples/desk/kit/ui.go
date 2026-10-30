package kit

import (
	_ "embed"
	"html"
	"strings"
)

//go:embed kit.css
var kitCSS string

//go:embed kit.js
var kitJS string

// CSS returns the desk UI kit stylesheet (light/dark, badges, fields).
func CSS() string { return kitCSS }

// JS returns the desk UI kit script (copy buttons, badges, theme toggle).
func JS() string { return kitJS }

// Page markers the shared contract test looks for in every tool's GET /.
// Tools get them for free by using the snippet helpers below.
const (
	MarkerCopyButton = "data-desk-copy"
	MarkerBadge      = "desk-badge"
	MarkerItemHeader = "desk-item-header"
	MarkerField      = "desk-field"
	MarkerAlert      = "data-desk-alert"
)

// CopyButton renders a copy button that flips to a "Copied" state via kit.js.
func CopyButton(label, text string) string {
	return `<button type="button" class="desk-copy" data-desk-copy data-copy="` +
		html.EscapeString(text) + `" data-copied="false">` + html.EscapeString(label) + `</button>`
}

// Badge renders a status badge for one of the shared statuses.
func Badge(status string) string {
	if !ValidStatus(status) {
		status = StatusPending
	}
	return `<span class="desk-badge" data-status="` + status + `">` + status + `</span>`
}

// ItemHeader renders the item header: id, status badge and share-link tools.
func ItemHeader(id, status, shareURL string) string {
	var b strings.Builder
	b.WriteString(`<div class="desk-item-header"><span class="desk-id">`)
	b.WriteString(html.EscapeString(id))
	b.WriteString(`</span>` + Badge(status) + `<span class="desk-tools" data-share="`)
	b.WriteString(html.EscapeString(shareURL))
	b.WriteString(`"></span></div>`)
	return b.String()
}

// TextField renders a labelled text/textarea/select field component.
func TextField(name, label, kind, value, recommended string) string {
	var b strings.Builder
	b.WriteString(`<div class="desk-field"><label>`)
	b.WriteString(html.EscapeString(label))
	if recommended != "" {
		b.WriteString(`<span class="desk-recommended">` + html.EscapeString(recommended) + `</span>`)
	}
	b.WriteString(`</label>`)
	switch kind {
	case "textarea":
		b.WriteString(`<textarea name="` + html.EscapeString(name) + `">` +
			html.EscapeString(value) + `</textarea>`)
	case "select":
		b.WriteString(`<select name="` + html.EscapeString(name) + `"></select>`)
	default:
		b.WriteString(`<input name="` + html.EscapeString(name) + `" value="` +
			html.EscapeString(value) + `">`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// Tool is one member of the desk family: a name (data dir, CLI, launcher
// tile, [data-desk-tool] accent selector), a label and an icon. The icon is
// the one the launcher tile uses, so tile and page top bar match.
type Tool struct {
	Name, Label, Icon string
}

// Tools is the desk family. Each has its own accent in kit.css
// (--desk-accent-<name>); everything else is shared.
var Tools = []Tool{
	{"notepad", "Notepad", "📝"},
	{"viewer", "Viewer", "👁"},
	{"whiteboard", "Whiteboard", "🎨"},
	{"brainstorm", "Brainstorm", "💡"},
	{"form", "Form", "📋"},
	{"chart", "Chart", "📊"},
}

// LookupTool returns the family entry for name; unknown names (fixtures,
// forks) get a neutral entry on the launcher primary accent.
func LookupTool(name string) Tool {
	for _, t := range Tools {
		if t.Name == name {
			return t
		}
	}
	label := name
	if label != "" {
		label = strings.ToUpper(label[:1]) + label[1:]
	}
	return Tool{Name: name, Label: label, Icon: "▣"}
}

// DataPalette is the kit's categorical data palette (light, then dark),
// mirrored by --desk-data-1..8 in kit.css. Chart series use it; the
// conformance check allowlists exactly these colours as user-content data.
var DataPalette = []string{
	"#2563eb", "#0d9488", "#d97706", "#db2777", "#7c3aed", "#65a30d", "#dc2626", "#0891b2",
	"#60a5fa", "#2dd4bf", "#fbbf24", "#f472b6", "#a78bfa", "#a3e635", "#f87171", "#22d3ee",
}

// Shell markers the conformance check looks for.
const (
	MarkerTokens = "desk-kit:tokens" // comment line at the top of kit.css
	MarkerAppBar = "desk-appbar"
)

// EmptyItemHeader is the item header before an item is open: a placeholder
// id, a pending badge and no share tools.
func EmptyItemHeader(placeholder string) string {
	return `<div class="desk-item-header"><span class="desk-id">` + html.EscapeString(placeholder) + `</span>` +
		Badge(StatusPending) + `<span class="desk-tools" data-share=""></span></div>`
}

// AppBar renders the one top bar every tool shows, in the same place:
// icon + tool name, then the item header (id, status badge, share/copy),
// optional tool-specific extras, then the theme toggle. itemHeader is
// ItemHeader/EmptyItemHeader output (or a tool's own .desk-item-header).
func AppBar(tool, homeURL, itemHeader, extra string) string {
	t := LookupTool(tool)
	if homeURL == "" {
		homeURL = "./"
	}
	var b strings.Builder
	b.WriteString(`<header class="desk-appbar" data-desk-shell>`)
	b.WriteString(`<a class="desk-brand" href="` + html.EscapeString(homeURL) + `"><span class="desk-icon" aria-hidden="true">` +
		t.Icon + `</span><span class="desk-name">` + html.EscapeString(t.Label) + `</span></a>`)
	b.WriteString(shellTools(itemHeader))
	if extra != "" {
		b.WriteString(`<span class="desk-appbar-extra">` + extra + `</span>`)
	}
	b.WriteString(ThemeToggle())
	b.WriteString(`</header>`)
	return b.String()
}

// shellTools puts the one shell action set (Copy link) into the item header.
// Tools never render their own; the link copies the header's data-share URL,
// or the page URL when none is given (scripts may update data-copy).
func shellTools(header string) string {
	share := ""
	if i := strings.Index(header, `data-share="`); i >= 0 {
		rest := header[i+len(`data-share="`):]
		if j := strings.IndexByte(rest, '"'); j >= 0 {
			share = html.UnescapeString(rest[:j])
		}
	}
	btn := `<span class="desk-tools">` + strings.Replace(CopyButton("Copy link", share), `<button `, `<button id="deskShareLink" `, 1) + `</span>`
	if i := strings.Index(header, `<span class="desk-tools"`); i >= 0 {
		if j := strings.Index(header[i:], `</span>`); j >= 0 {
			return header[:i] + btn + header[i+j+len(`</span>`):]
		}
	}
	if i := strings.LastIndex(header, `</div>`); i >= 0 {
		return header[:i] + btn + header[i:]
	}
	return header + btn
}

// ThemeToggle is the shared light/dark switch (also bound to "t").
func ThemeToggle() string {
	return `<button type="button" class="desk-icon-btn" data-desk-theme aria-label="Toggle theme" title="Toggle theme (t)">◐</button>`
}

// State renders the shared empty / error / loading block.
func State(kind, title, hint string) string {
	switch kind {
	case "empty", "error", "loading":
	default:
		kind = "empty"
	}
	role := ""
	if kind == "error" {
		role = ` role="alert"`
	}
	s := `<div class="desk-state" data-state="` + kind + `"` + role + `><div class="desk-state-title">` + html.EscapeString(title) + `</div>`
	if hint != "" {
		s += `<div class="desk-muted">` + html.EscapeString(hint) + `</div>`
	}
	return s + `</div>`
}

// Rendered marks text for the kit renderer (deskRender): kit.js POSTs the
// element's text to the tool's /api/render endpoint and swaps in the
// sanitized server HTML, styled by .desk-content. mode is "block" or
// "inline" (a hint sent with the request).
func Rendered(tag, mode, text string) string {
	if mode != "inline" {
		mode = "block"
	}
	return `<` + tag + ` class="desk-content" data-desk-render="` + mode + `">` + html.EscapeString(text) + `</` + tag + `>`
}

// Page wraps body in the desk page: kit CSS/JS and the tool's family
// accent ([data-desk-tool]). body normally starts with AppBar. kit.js loads
// in <head> so the saved theme applies before first paint and tool scripts
// in body can call deskRender/deskToast.
func Page(tool, title, body string) []byte {
	return []byte(`<!doctype html><html lang="en" data-desk-tool="` + html.EscapeString(tool) + `"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + html.EscapeString(title) + `</title><style>` + kitCSS + `</style><script>` + kitJS + `</script></head>` +
		`<body class="desk-app">` + body + `</body></html>`)
}

// Shell wraps body in a desk page without a family accent. Prefer Page.
func Shell(title, body string) []byte { return Page("", title, body) }
