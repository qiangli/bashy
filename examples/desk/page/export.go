// Export: the organizer's session record. Markdown is a repo-committable
// record (title, session metadata, every block's final state inline in body
// order, participants by handle and auth only); HTML is the same record as
// one self-contained page; PDF prints that HTML through headless Chrome.
// Addresses, fingerprints, cookies, respondent tokens and facilitator keys
// never appear in any format.
package main

import (
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	kit "github.com/qiangli/bashy/examples/desk/kit"
)

// export serves GET /api/export?id=&to=md|markdown|html|pdf (organizer only).
func (a *app) export(w http.ResponseWriter, r *http.Request, id, to string) {
	it := a.loadOr404(w, id)
	if it == nil {
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, http.StatusForbidden, "Only the organizer can export this page")
		return
	}
	_, done, ok := a.knownParticipantLocked(w, r, it)
	if !ok {
		return
	}
	defer done()
	switch to {
	case "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		io.WriteString(w, a.exportMarkdownLocked(it))
	case "html":
		body, err := a.exportHTMLLocked(it, r)
		if err != nil {
			kit.Fail(w, http.StatusInternalServerError, "Could not render")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(body)
	case "pdf":
		body, err := a.exportHTMLLocked(it, r)
		if err != nil {
			kit.Fail(w, http.StatusInternalServerError, "Could not render")
			return
		}
		tmp, err := os.MkdirTemp("", "page-pdf-")
		if err != nil {
			kit.Fail(w, http.StatusInternalServerError, "Could not prepare PDF export")
			return
		}
		defer os.RemoveAll(tmp)
		out := filepath.Join(tmp, "export.pdf")
		if err := printPDFBrowser(body, out); err != nil {
			kit.Fail(w, http.StatusInternalServerError, "Could not prepare PDF export")
			return
		}
		data, err := os.ReadFile(out)
		if err != nil || len(data) == 0 {
			kit.Fail(w, http.StatusInternalServerError, "Could not prepare PDF export")
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(data)
	default:
		kit.Fail(w, http.StatusBadRequest, "Export to md, html or pdf (--to md|html|pdf)")
	}
}

// exportHTMLLocked renders the record face of every block plus the
// participants section as one self-contained kit page.
func (a *app) exportHTMLLocked(it *item, r *http.Request) ([]byte, error) {
	body, err := renderPage(it, recordFace, func(b *block) string {
		if b.Kind == "embed" && b.Ref != "" {
			label := b.Options["title"]
			if label == "" {
				label = b.ID
			}
			return `<section class="desk-panel page-block" data-block="` + html.EscapeString(b.ID) +
				`" data-kind="embed"><p><a href="` + html.EscapeString(b.Ref) + `">` +
				html.EscapeString(label) + `</a></p></section>`
		}
		return a.renderBlockFaces(r, it, b, recordFace)
	})
	if err != nil {
		return nil, err
	}
	return kit.Page(tool, it.Title, `<main class="desk-main">`+body+participantsHTML(it)+`</main>`), nil
}

func exportMarkdown(it *item) string { return exportMarkdownWith(it, blockMarkdown) }

func (a *app) exportMarkdownLocked(it *item) string {
	return exportMarkdownWith(it, func(b *block) string { return a.blockMarkdownLocked(it, b) })
}

func sessionMetadataLine(it *item) string {
	return "> Page `" + it.ID + "` · status: " + it.Status + " · audience: " + it.Audience +
		" · revision: " + strconv.Itoa(it.Revision)
}

func exportMarkdownWith(it *item, blockMD func(*block) string) string {
	var body strings.Builder
	segs, _ := parseBody(it.Body)
	for _, s := range segs {
		if s.Block != nil {
			body.WriteString(blockMD(s.Block))
			if !strings.HasSuffix(body.String(), "\n") {
				body.WriteString("\n")
			}
		} else {
			body.WriteString(s.Text)
		}
	}
	out := body.String()
	meta := sessionMetadataLine(it)
	if titleOf(it.Body) == "" {
		out = "# " + it.Title + "\n\n" + meta + "\n\n" + out
	} else if idx := strings.Index(out, "\n"); idx >= 0 {
		out = out[:idx+1] + "\n" + meta + "\n" + out[idx+1:]
	} else {
		out += "\n" + meta + "\n"
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	var b strings.Builder
	b.WriteString(out)
	if rows := exportedParticipants(it); len(rows) > 0 {
		b.WriteString("\n## Participants\n\n")
		for _, p := range rows {
			b.WriteString("- " + escapeParticipantMarkdown(p.Handle) + " (" + p.Auth + ")\n")
		}
		if breakMode == "export-addresses" {
			for _, p := range it.Participants {
				if p != nil {
					b.WriteString(p.Address + "\n")
				}
			}
		}
	}
	return b.String()
}

func participantsListHTML(it *item) string {
	rows := exportedParticipants(it)
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ul>`)
	for _, p := range rows {
		b.WriteString(`<li>` + html.EscapeString(p.Handle) + ` <span class="desk-muted">` + html.EscapeString(p.Auth) + `</span></li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

func participantsHTML(it *item) string {
	rows := exportedParticipants(it)
	if len(rows) == 0 {
		return ""
	}
	return `<section class="desk-panel"><h2 class="desk-h2">Participants</h2>` + participantsListHTML(it) + `</section>`
}

// contentMarkdown is a book or note block's place in the record: the
// sibling's Markdown inline when reachable, a graceful fallback otherwise.
func contentMarkdown(b *block) string {
	base, id, pageSel, what := notepadBase(), b.Ref, "", "Note "+b.Ref
	if b.Kind == "book" {
		id, pageSel = parseBookRef(b.Ref)
		base, what = viewerBase(), "Book "+b.Ref
	}
	obj, err := siblingShow(base, id, pageSel)
	if err != nil {
		return blockMarkdown(b) + "\n_" + what + " is " + err.Error() + "._\n"
	}
	title, _ := obj["title"].(string)
	md, _ := obj["body"].(string)
	if pg, ok := obj["page"].(map[string]any); ok {
		md, _ = pg["markdown"].(string)
	} else if items, ok := obj["items"].([]any); ok && len(items) > 0 {
		first, _ := items[0].(map[string]any)
		md, _ = first["markdown"].(string)
	}
	if strings.TrimSpace(md) == "" {
		return blockMarkdown(b) + "\n_" + what + " is not available._\n"
	}
	var s strings.Builder
	heading := title
	if heading == "" {
		heading = what
	}
	s.WriteString("### " + heading + "\n\n")
	s.WriteString(md)
	if !strings.HasSuffix(s.String(), "\n") {
		s.WriteString("\n")
	}
	return s.String()
}

// embedMarkdown is an embed block's place in the record: its public link
// only. Organizer and display URLs live in private face settings and are
// never emitted.
func embedMarkdown(b *block) string {
	if b.Ref == "" {
		return blockMarkdown(b)
	}
	label := b.Options["title"]
	if label == "" {
		label = b.ID
	}
	return "> **embed** `" + b.ID + "` [" + escapeParticipantMarkdown(label) + "](" + b.Ref + ")\n"
}

// hereMarkdown is a presence block's place in the record: the participant
// list (handle and auth only) when anyone is present.
func hereMarkdown(it *item, b *block) string {
	rows := exportedParticipants(it)
	if len(rows) == 0 {
		return blockMarkdown(b)
	}
	var s strings.Builder
	s.WriteString(blockMarkdown(b))
	s.WriteString("\n")
	for _, p := range rows {
		s.WriteString("- " + escapeParticipantMarkdown(p.Handle) + " (" + p.Auth + ")\n")
	}
	return s.String()
}
