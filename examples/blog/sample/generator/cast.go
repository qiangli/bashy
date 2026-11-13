// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// CastTranscript renders an asciicast v2 file (the format the cast family
// records and every terminal player reads) into a static, colored terminal
// transcript. No JavaScript, no player download: the scene is readable on
// the page exactly as the terminal showed it, pauses collapsed.
//
// The path may be absolute, or relative to the site's media/ directory.
func CastTranscript(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !filepath.IsAbs(path) {
			for _, base := range []string{"media", filepath.Join("static", "media")} {
				if alt, err2 := os.ReadFile(filepath.Join(base, path)); err2 == nil {
					data = alt
					err = nil
					break
				}
			}
		}
		if err != nil {
			return "", fmt.Errorf("cast: cannot read %s: %w", filepath.Base(path), err)
		}
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 1 {
		return "", fmt.Errorf("cast: empty file")
	}

	var header struct {
		Version  int     `json:"version"`
		Width    int     `json:"width"`
		Height   int     `json:"height"`
		Duration float64 `json:"duration"`
		Title    string  `json:"title"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil || header.Version != 2 {
		return "", fmt.Errorf("cast: not an asciicast v2 file")
	}

	screen := newTermScreen(header.Width, header.Height)
	events := 0
	var lastTime float64
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event []json.RawMessage
		if err := json.Unmarshal([]byte(line), &event); err != nil || len(event) < 2 {
			continue
		}
		var kind string
		if err := json.Unmarshal(event[1], &kind); err != nil {
			continue
		}
		if kind != "o" {
			continue
		}
		var t float64
		_ = json.Unmarshal(event[0], &t)
		var text string
		if err := json.Unmarshal(event[2], &text); err != nil {
			continue
		}
		if t-lastTime > 2.0 && screen.rowLen() > 0 {
			screen.write("\n")
		}
		lastTime = t
		events++
		screen.write(text)
	}

	title := header.Title
	if title == "" {
		title = "Terminal scene"
	}
	meta := fmt.Sprintf("%d events · %.1fs · %dx%d", events, header.Duration, header.Width, header.Height)
	return fmt.Sprintf("<figure class=\"cast\">\n<figcaption class=\"cast-title\">%s</figcaption>\n<pre class=\"cast-screen\"><code>%s</code></pre>\n<div class=\"cast-meta\">%s</div>\n</figure>\n",
		escapeHTML(title), screen.html(), escapeHTML(meta)), nil
}

// termScreen is a tiny ANSI-aware terminal: enough of a VT to replay a
// recorded scene (SGR colors, carriage returns, line feeds, backspace,
// tab) without pretending to be one.
type termScreen struct {
	cols, rows               int
	lines                    [][]termChar
	curRow                   int
	curCol                   int
	fg, bg                   string
	bold, underline, inverse bool
}

type termChar struct {
	ch                       rune
	fg, bg                   string
	bold, underline, inverse bool
}

func newTermScreen(cols, rows int) *termScreen {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	s := &termScreen{cols: cols, rows: rows}
	s.lines = [][]termChar{{}}
	return s
}

func (s *termScreen) rowLen() int {
	if s.curRow < len(s.lines) {
		return len(s.lines[s.curRow])
	}
	return 0
}

func (s *termScreen) newline() {
	s.curRow++
	s.curCol = 0
	for len(s.lines) <= s.curRow {
		s.lines = append(s.lines, []termChar{})
	}
}

func (s *termScreen) put(ch rune) {
	for len(s.lines) <= s.curRow {
		s.lines = append(s.lines, []termChar{})
	}
	row := s.lines[s.curRow]
	for len(row) <= s.curCol {
		row = append(row, termChar{ch: ' '})
	}
	row[s.curCol] = termChar{ch: ch, fg: s.fg, bg: s.bg, bold: s.bold, underline: s.underline, inverse: s.inverse}
	s.lines[s.curRow] = row
	s.curCol++
}

func (s *termScreen) write(text string) {
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if size == 0 {
			break
		}
		if r == 0x1b {
			i += s.escape(text[i:])
			continue
		}
		i += size

		switch r {
		case '\n':
			s.newline()
		case '\r':
			s.curCol = 0
		case '\b':
			if s.curCol > 0 {
				s.curCol--
			}
		case '\t':
			for j := 0; j < 4; j++ {
				s.put(' ')
			}
		case 0x07: // bell
		default:
			s.put(r)
		}
	}
}

// escape handles one control sequence starting at ESC; returns bytes eaten.
func (s *termScreen) escape(text string) int {
	if len(text) < 2 {
		return len(text)
	}
	switch text[1] {
	case '[':
		j := 2
		for j < len(text) && (text[j] == ';' || text[j] == '?' || (text[j] >= '0' && text[j] <= '9')) {
			j++
		}
		if j >= len(text) {
			return len(text)
		}
		final := text[j]
		params := text[2:j]
		if final == 'm' {
			s.sgr(params)
		}
		// cursor movement, erase and other CSI: ignored (transcript keeps
		// what was printed)
		return j + 1
	case ']':
		// OSC (window title etc.): skip until BEL or ST
		if k := strings.IndexByte(text, 0x07); k >= 0 {
			return k + 1
		}
		if k := strings.Index(text, "\x1b\\"); k >= 0 {
			return k + 2
		}
		return len(text)
	default:
		// two-byte escapes: ignore
		return 2
	}
}

func (s *termScreen) sgr(params string) {
	if params == "" {
		params = "0"
	}
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		n := 0
		fmt.Sscanf(parts[i], "%d", &n)
		switch {
		case n == 0:
			s.fg, s.bg = "", ""
			s.bold, s.underline, s.inverse = false, false, false
		case n == 1:
			s.bold = true
		case n == 4:
			s.underline = true
		case n == 7:
			s.inverse = true
		case n == 22:
			s.bold = false
		case n == 24:
			s.underline = false
		case n == 27:
			s.inverse = false
		case n >= 30 && n <= 37, n >= 90 && n <= 97:
			s.fg = ansiColor(n)
		case n == 39:
			s.fg = ""
		case n >= 40 && n <= 47, n >= 100 && n <= 107:
			s.bg = ansiColor(n - 10)
		case n == 49:
			s.bg = ""
		case n == 38 || n == 48:
			// 256-color and truecolor: consume the sub-params
			if i+1 < len(parts) && parts[i+1] == "5" && i+2 < len(parts) {
				i += 2
			} else if i+1 < len(parts) && parts[i+1] == "2" && i+4 < len(parts) {
				i += 4
			}
		}
	}
}

func ansiColor(n int) string {
	names := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}
	switch {
	case n >= 30 && n <= 37:
		return names[n-30]
	case n >= 90 && n <= 97:
		return "bright-" + names[n-90]
	default:
		return ""
	}
}

func (s *termScreen) html() string {
	var b strings.Builder
	for _, row := range s.lines {
		prev := termChar{}
		run := strings.Builder{}
		flush := func() {
			if run.Len() == 0 {
				return
			}
			cls := termClass(prev)
			if cls == "" {
				b.WriteString(escapeHTML(run.String()))
			} else {
				fmt.Fprintf(&b, "<span class=\"%s\">%s</span>", cls, escapeHTML(run.String()))
			}
			run.Reset()
		}
		for _, ch := range row {
			if ch.fg != prev.fg || ch.bold != prev.bold || ch.underline != prev.underline || ch.inverse != prev.inverse {
				flush()
				prev = ch
			}
			run.WriteRune(ch.ch)
		}
		flush()
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func termClass(c termChar) string {
	var parts []string
	if c.fg != "" {
		parts = append(parts, "fg-"+c.fg)
	}
	if c.bold {
		parts = append(parts, "bold")
	}
	if c.underline {
		parts = append(parts, "underline")
	}
	if c.inverse {
		parts = append(parts, "inverse")
	}
	return strings.Join(parts, " ")
}
