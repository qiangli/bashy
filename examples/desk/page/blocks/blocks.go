// Package blocks holds one small renderer per page block kind, each backed
// by a template under templates/. A shaping wish that adds a kind adds one
// renderer function and one template file here and registers the kind in
// the page's block registry (blockKinds) and face renderer.
package blocks

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates/*.html
var templatesFS embed.FS

var tmpl = template.Must(template.ParseFS(templatesFS, "templates/*.html"))

func render(name string, data any) string {
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, name, data); err != nil {
		return ""
	}
	return b.String()
}

// ContentData is the view model for book.html and note.html: another app's
// Markdown, already rendered by the page's sanitizing renderer.
type ContentData struct {
	Ref       string
	Title     string
	PageTitle string
	Body      template.HTML
	Err       string
}

// EmbedData is the view model for embed.html.
type EmbedData struct {
	Title     string
	Src       string
	Sandbox   string
	Private   bool // organizer face: start only in an isolated iframe
	Frame     bool
	Link      bool
	LinkLabel string
}

// TimerData is the view model for timer.html.
type TimerData struct {
	ID        string
	Name      string
	Display   string
	State     string
	Running   bool
	EndsAt    int64
	Controls  bool
	Projected bool
}

// FormatClock renders seconds as M:SS (or H:MM:SS at an hour or more).
func FormatClock(total int) string {
	if total < 0 {
		total = 0
	}
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func Book(d ContentData) string { return render("book.html", d) }
func Note(d ContentData) string { return render("note.html", d) }
func Embed(d EmbedData) string  { return render("embed.html", d) }
func Timer(d TimerData) string  { return render("timer.html", d) }
