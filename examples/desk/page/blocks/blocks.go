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

type HereData struct {
	Rows []HereRow
}

type HereRow struct {
	Handle, Auth, LastSeen, Address, Fingerprint string
	Online                                       bool
}

type HandsData struct {
	Handles           []string
	Organizer         bool
	ParticipantAction string
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
func Here(d HereData) string    { return render("here.html", d) }
func Hands(d HandsData) string  { return render("hands.html", d) }

// BoardData is the view model for board.html: a brainstorm session read
// over HTTP, shaped for one face.
type BoardData struct {
	ID         string
	Title      string
	Phase      string
	Round      int
	Columns    []BoardColumn
	Enrolled   bool // a page participant (or the organizer's preview)
	Compose    bool // write phase: add, edit and delete own cards
	Vote       bool // vote phase: toggle votes within the budget
	VotesLeft  int
	VoteBudget int
	Total      int
	LinkURL    string // organizer face only: the session in brainstorm
	LinkLabel  string
	Err        string
}

// BoardColumn is one brainstorm column with its visible cards.
type BoardColumn struct {
	Index  int
	Name   string
	Prompt string
	Cards  []BoardCard
}

// BoardCard is one visible card; Mine and Voted are the caller's.
type BoardCard struct {
	ID    string
	Text  string
	Name  string
	Votes int
	Mine  bool
	Voted bool
}

func Board(d BoardData) string { return render("board.html", d) }
