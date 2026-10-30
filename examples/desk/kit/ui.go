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
	b.WriteString(`</span>` + Badge(status) + `<span class="desk-tools">`)
	b.WriteString(CopyButton("Copy link", shareURL))
	b.WriteString(`</span></div>`)
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

// Shell wraps body in a minimal desk page that wires in the kit CSS/JS.
func Shell(title, body string) []byte {
	return []byte(`<!doctype html><html lang="en"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + html.EscapeString(title) + `</title><style>` + kitCSS + `</style></head><body>` +
		body + `<script>` + kitJS + `</script></body></html>`)
}
