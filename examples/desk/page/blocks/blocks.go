// Package blocks holds one small renderer per page block kind, each backed
// by a template under templates/. A shaping wish that adds a kind adds one
// renderer function and one template file here and registers the kind in
// the page's block registry.
package blocks

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed templates/*.html
var templatesFS embed.FS

var tmpl = template.Must(template.ParseFS(templatesFS, "templates/*.html"))

func render(name string, data any) (string, error) {
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, name, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// BookData is the view model for book.html.
type BookData struct {
	ID        string
	Title     string
	PageTitle string
	Body      template.HTML
	Err       string
}

// NoteData is the view model for note.html.
type NoteData struct {
	ID    string
	Title string
	Body  template.HTML
	Err   string
}

// EmbedData is the view model for embed.html.
type EmbedData struct {
	ID    string
	Src   string
	Title string
}

// TimerData is the view model for timer.html.
type TimerData struct {
	ID        string
	Name      string
	Display   string
	State     string
	Remaining int
	Running   bool
	EndsAt    int64
	Organizer bool
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

func Book(d BookData) (string, error)   { return render("book.html", d) }
func Note(d NoteData) (string, error)   { return render("note.html", d) }
func Embed(d EmbedData) (string, error) { return render("embed.html", d) }
func Timer(d TimerData) (string, error) { return render("timer.html", d) }

// Kinds lists the kinds rendered by this package.
func Kinds() []string { return []string{"book", "embed", "note", "timer"} }

// ValidKind reports whether kind is rendered here.
func ValidKind(kind string) bool {
	return strings.Contains(" book embed note timer ", " "+kind+" ")
}
