// Blocks: the ```block fences of a page body. A block is one line,
// `kind:ref [key=value ...]`, either as the fence's info string
// (```block poll:form-9f3a) or as the first line of its body. The page
// skeleton parses, validates and places blocks; each kind's live renderer
// (native adapter, embed, timer, ...) lands with its own story and
// registers here.
package main

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"

	kit "desk-kit"
)

// blockKind describes one block kind: whether it names an item another app
// owns (ref), and the faces it renders (organizer, participant, display).
type blockKind struct {
	// Ref: "required", "optional" or "none".
	Ref string
	// Embed refs are URLs (same-origin path or http(s)), not item ids.
	Embed bool
	// Faces the kind renders; dual-view kinds have an organizer face.
	Faces []string
}

// blockKinds is the registry of known kinds (docs/page-live-group-page.md).
// A shaping wish that adds a kind adds one entry and one renderer.
var blockKinds = map[string]blockKind{
	"poll":  {Ref: "required", Faces: []string{"organizer", "participant", "display"}},
	"form":  {Ref: "required", Faces: []string{"organizer", "participant"}},
	"book":  {Ref: "required", Faces: []string{"participant"}},
	"note":  {Ref: "required", Faces: []string{"participant"}},
	"board": {Ref: "required", Faces: []string{"organizer", "participant"}},
	"chart": {Ref: "required", Faces: []string{"participant"}},
	"timer": {Ref: "optional", Faces: []string{"organizer", "participant", "display"}},
	"here":  {Ref: "none", Faces: []string{"organizer", "participant"}},
	"hands": {Ref: "none", Faces: []string{"organizer", "participant"}},
	"embed": {Ref: "optional", Embed: true, Faces: []string{"organizer", "participant", "display"}},
}

// block is one parsed ```block fence.
type block struct {
	ID      string            `json:"id"`
	Kind    string            `json:"kind"`
	Ref     string            `json:"ref,omitempty"`
	Options map[string]string `json:"options,omitempty"`
}

// segment is a run of Markdown text or one block, in body order.
type segment struct {
	Text  string
	Block *block
}

var (
	refRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_./:-]{0,127}$`)
	optKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
)

func kindNames() string {
	names := make([]string, 0, len(blockKinds))
	for k := range blockKinds {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// fenceOpen reports a fence opener: its marker run and info string.
func fenceOpen(line string) (marker, info string, ok bool) {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 || len(t) < 3 {
		return "", "", false
	}
	c := t[0]
	if c != '`' && c != '~' {
		return "", "", false
	}
	n := 0
	for n < len(t) && t[n] == c {
		n++
	}
	if n < 3 {
		return "", "", false
	}
	info = strings.TrimSpace(t[n:])
	if c == '`' && strings.Contains(info, "`") {
		return "", "", false
	}
	return t[:n], info, true
}

// fenceClose reports whether line closes a fence opened with marker.
func fenceClose(line, marker string) bool {
	t := strings.TrimSpace(line)
	if len(t) < len(marker) || t[0] != marker[0] {
		return false
	}
	return strings.Trim(t, string(marker[0])) == ""
}

// parseBody splits a page body into Markdown segments and blocks. Fences
// that are not ```block stay Markdown (their contents are never parsed).
func parseBody(body string) ([]segment, error) {
	var segs []segment
	var text strings.Builder
	seen := map[string]bool{}
	count := map[string]int{}
	lines := strings.SplitAfter(body, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		marker, info, ok := fenceOpen(strings.TrimRight(line, "\r\n"))
		if !ok {
			text.WriteString(line)
			continue
		}
		word, rest, _ := strings.Cut(info, " ")
		if word != "block" {
			// An ordinary fence: copy it through to its close verbatim.
			text.WriteString(line)
			for i++; i < len(lines); i++ {
				text.WriteString(lines[i])
				if fenceClose(strings.TrimRight(lines[i], "\r\n"), marker) {
					break
				}
			}
			continue
		}
		spec := strings.TrimSpace(rest)
		closed := false
		for i++; i < len(lines); i++ {
			l := strings.TrimSpace(lines[i])
			if fenceClose(l, marker) {
				closed = true
				break
			}
			if l == "" {
				continue
			}
			if spec != "" {
				return nil, fmt.Errorf("block %q: one line per block (kind:ref key=value ...)", spec)
			}
			spec = l
		}
		if !closed {
			return nil, fmt.Errorf("block fence opened on line %d is not closed", lineNo(lines, i))
		}
		b, err := parseBlock(spec)
		if err != nil {
			return nil, err
		}
		count[b.Kind]++
		if b.ID == "" {
			b.ID = fmt.Sprintf("%s-%d", b.Kind, count[b.Kind])
		}
		if seen[b.ID] {
			return nil, fmt.Errorf("block id %q is used twice; give one of them id=NAME", b.ID)
		}
		seen[b.ID] = true
		if text.Len() > 0 {
			segs = append(segs, segment{Text: text.String()})
			text.Reset()
		}
		segs = append(segs, segment{Block: b})
	}
	if text.Len() > 0 {
		segs = append(segs, segment{Text: text.String()})
	}
	return segs, nil
}

func lineNo(lines []string, i int) int {
	if i > len(lines) {
		return len(lines)
	}
	return i
}

// parseBlock parses `kind:ref [key=value ...]`.
func parseBlock(spec string) (*block, error) {
	fields := strings.Fields(spec)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty block: write kind:ref, e.g. poll:form-9f3a")
	}
	kind, ref, _ := strings.Cut(fields[0], ":")
	k, ok := blockKinds[kind]
	if !ok {
		return nil, fmt.Errorf("unknown block kind %q (known: %s)", kind, kindNames())
	}
	b := &block{Kind: kind, Ref: ref}
	switch {
	case k.Ref == "required" && ref == "":
		return nil, fmt.Errorf("block %s needs a ref: %s:REF", kind, kind)
	case k.Ref == "none" && ref != "":
		return nil, fmt.Errorf("block %s takes no ref", kind)
	case ref != "" && k.Embed:
		if err := validEmbedRef(ref); err != nil {
			return nil, err
		}
		if hasOrganizerParameter(ref, false) {
			return nil, fmt.Errorf("block embed: put the organizer URL in private face settings")
		}
	case ref != "" && !refRe.MatchString(ref):
		return nil, fmt.Errorf("block %s: ref %q: use letters, digits, - _ . / :", kind, ref)
	}
	for _, f := range fields[1:] {
		key, val, ok := strings.Cut(f, "=")
		if !ok || !optKeyRe.MatchString(key) {
			return nil, fmt.Errorf("block %s: option %q: write key=value", kind, f)
		}
		if privateParameter(key, true) || key == "organizer-url" || key == "display-url" {
			return nil, fmt.Errorf("block %s: put private face settings outside the page body", kind)
		}
		if key == "id" {
			if !kit.ValidName(val) {
				return nil, fmt.Errorf("block id %q: use 1-64 letters, digits, - or _", val)
			}
			b.ID = val
			continue
		}
		if b.Options == nil {
			b.Options = map[string]string{}
		}
		b.Options[key] = val
	}
	return b, nil
}

// validEmbedRef accepts a same-origin path or an http(s) URL; nothing that
// a browser would execute or resolve elsewhere (javascript:, //host).
func validEmbedRef(ref string) error {
	if strings.ContainsAny(ref, "\\\t\r\n ") {
		return fmt.Errorf("block embed: invalid URL")
	}
	if strings.HasPrefix(ref, "/") && !strings.HasPrefix(ref, "//") {
		if _, err := url.Parse(ref); err == nil {
			return nil
		}
	}
	if u, err := url.Parse(ref); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil {
		return nil
	}
	return fmt.Errorf("block embed: use a same-origin path (/...) or an http(s) URL")
}

// blocksOf lists the blocks of a body (already validated on save).
func blocksOf(body string) []block {
	segs, err := parseBody(body)
	if err != nil {
		return nil
	}
	out := []block{}
	for _, s := range segs {
		if s.Block != nil {
			out = append(out, *s.Block)
		}
	}
	return out
}

// blockHTML is the block's place on the page. Until a kind's renderer
// lands it is a labelled placeholder carrying the block id, so the live
// stream ({item, block, revision}) can address it.
func blockHTML(b *block) string {
	label := b.Kind
	if b.Ref != "" {
		label += ":" + b.Ref
	}
	return `<section class="desk-panel page-block" id="block-` + html.EscapeString(b.ID) +
		`" data-block="` + html.EscapeString(b.ID) + `" data-kind="` + html.EscapeString(b.Kind) + `">` +
		`<h2 class="desk-h2">` + html.EscapeString(b.Kind) + ` <code>` + html.EscapeString(b.ID) + `</code></h2>` +
		`<p class="desk-muted">` + html.EscapeString(label) + ` · live block</p></section>`
}

// blockMarkdown is the block's line in the Markdown record.
func blockMarkdown(b *block) string {
	ref := ""
	if b.Ref != "" {
		ref = " `" + b.Ref + "`"
	}
	return "> **" + b.Kind + "** " + "`" + b.ID + "`" + ref + "\n"
}
