package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	kit "desk-kit"
)

// book is one viewer item: an ordered, append-only list of pages addressed
// by a stable id. A sprint book (sprint-N) outlives any one manager: the
// next manager appends to the same id. A session book groups one session's
// reports. Pages are never edited; a correction is the next page.
type book struct {
	Version  int             `json:"version"`
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Sprint   int             `json:"sprint,omitempty"`
	Session  string          `json:"session,omitempty"`
	Status   string          `json:"status"`
	Revision int             `json:"revision"`
	Created  string          `json:"created"`
	Updated  string          `json:"updated"`
	Owner    string          `json:"owner"`
	Members  map[string]bool `json:"members"`
	Invites  map[string]bool `json:"invites"`
	Fields   []kit.Field     `json:"fields,omitempty"`
	Pages    []page          `json:"pages"`
}

// page is one report in a book. Markdown is the filled source the kit
// renderer turns into HTML at view time; Template and Data record how it
// was made, so a later template change never rewrites history.
type page struct {
	N        int             `json:"n"`
	Title    string          `json:"title"`
	Template string          `json:"template,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Markdown string          `json:"markdown"`
	Assets   []string        `json:"assets,omitempty"`
	Created  string          `json:"created"`
}

func (p page) id() string { return "p" + strconv.Itoa(p.N) }

func bookPath(dir, id string) string { return filepath.Join(dir, "items", id+".json") }

// assetDir is one page's private asset directory: embeds resolve here
// only, so a later page's file of the same name never changes history.
func assetDir(dir, id string, n int) string {
	return filepath.Join(dir, "items", id, "assets", strconv.Itoa(n))
}

func readBook(dir, id string) (*book, error) {
	if !kit.ValidName(id) {
		return nil, fmt.Errorf("use 1-64 letters, digits, - or _ for an id")
	}
	raw, err := os.ReadFile(bookPath(dir, id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	b := &book{}
	if err := json.Unmarshal(raw, b); err != nil {
		return nil, fmt.Errorf("saved book %q is invalid; preserve it for recovery", id)
	}
	if b.Version != 1 || b.ID != id || !kit.ValidStatus(b.Status) {
		return nil, fmt.Errorf("unsupported or invalid saved book %q", id)
	}
	if b.Members == nil {
		b.Members = map[string]bool{}
	}
	if b.Invites == nil {
		b.Invites = map[string]bool{}
	}
	return b, nil
}

// pageIndex resolves a page selector: "", "last", "first", "N", "pN".
// It returns the 0-based index, or -1 when the selector names no page.
func (b *book) pageIndex(sel string) int {
	if len(b.Pages) == 0 {
		return -1
	}
	switch strings.ToLower(strings.TrimSpace(sel)) {
	case "", "last":
		return len(b.Pages) - 1
	case "first":
		return 0
	}
	s := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(sel)), "p")
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > len(b.Pages) {
		return -1
	}
	return n - 1
}

// prevFor is the page page i is compared with: the latest earlier page of
// the same kind (same template, or both plain Markdown), so a status
// report is compared with the previous status report, not with a note in
// between. -1 means there is nothing comparable.
func (b *book) prevFor(i int) int {
	for j := i - 1; j >= 0; j-- {
		if b.Pages[j].Template == b.Pages[i].Template {
			return j
		}
	}
	return -1
}

// Changed-since-previous: pages are compared block by block. A block is a
// run of lines between blank lines (fences and indented continuations stay
// whole); a block of the current page that does not occur in the previous
// page is new or changed.

var (
	fenceRe  = regexp.MustCompile("^\\s{0,3}(`{3,}|~{3,})")
	refDefRe = regexp.MustCompile(`^\s{0,3}\[[^\]^][^\]]*\]:\s*\S`)
	// Documents whose rendering spans blocks (footnotes, a TOC) render
	// whole; the change banner still reports the counts.
	wholeRe = regexp.MustCompile(`(?im)\[\^[^\]]+\]|^\s*\[?\[toc\]\]?\s*$`)
)

// splitBlocks splits Markdown into top-level blocks.
func splitBlocks(md string) []string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var blocks []string
	var cur []string
	fence := ""
	flush := func() {
		if len(cur) > 0 {
			blocks = append(blocks, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for i, ln := range lines {
		if fence != "" {
			cur = append(cur, ln)
			if m := fenceRe.FindStringSubmatch(ln); m != nil && m[1][0] == fence[0] && len(m[1]) >= len(fence) &&
				strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(ln), string(fence[0]))) == "" {
				fence = ""
			}
			continue
		}
		if m := fenceRe.FindStringSubmatch(ln); m != nil {
			fence = m[1]
			cur = append(cur, ln)
			continue
		}
		if strings.TrimSpace(ln) == "" {
			// An indented line after the blank continues the block (a
			// loose list item's paragraph, nested content).
			next := ""
			for _, l := range lines[i+1:] {
				if strings.TrimSpace(l) != "" {
					next = l
					break
				}
			}
			if len(cur) > 0 && (strings.HasPrefix(next, "  ") || strings.HasPrefix(next, "\t")) {
				cur = append(cur, ln)
				continue
			}
			flush()
			continue
		}
		cur = append(cur, ln)
	}
	flush()
	return blocks
}

func normBlock(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.Join(lines, "\n")
}

// diffBlocks marks each current block changed when it is absent from the
// previous page (multiset match) and counts previous blocks that are gone.
func diffBlocks(prev, cur []string) (changed []bool, removed int) {
	pool := map[string]int{}
	for _, b := range prev {
		pool[normBlock(b)]++
	}
	changed = make([]bool, len(cur))
	for i, b := range cur {
		k := normBlock(b)
		if pool[k] > 0 {
			pool[k]--
		} else {
			changed[i] = true
		}
	}
	for _, n := range pool {
		removed += n
	}
	return changed, removed
}
