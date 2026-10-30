package main

// Templates: the agent gives DATA (JSON + short Markdown prose fields), the
// viewer fills a shipped, shapable Go text/template into Markdown, and the
// kit renderer turns that into sanitized HTML. The agent never writes HTML.
//
// Each template is two files: templates/NAME.md.tmpl and its data shape,
// templates/NAME.schema.json (a small JSON-schema subset). Data is
// validated before anything renders; a mismatch is refused with the path
// of the first bad value, never rendered broken. Shaped copies under
// <data dir>/templates/ override the shipped ones.
//
// Escaping: every string reaches the template as mdText, whose String()
// backslash-escapes all Markdown punctuation, so a value prints as the
// literal text it is (no emphasis, links, embeds or HTML). Properties the
// schema marks "format": "markdown" are prose: Markdown is honoured but raw
// HTML is shown as text. The only other way out is the json func, which
// emits JSON with <, >, & and backticks escaped (for ```chart fences).

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	kit "desk-kit"
)

//go:embed templates
var shippedTemplates embed.FS

// schema is the supported JSON-schema subset.
type schema struct {
	Type                 string             `json:"type"`
	Description          string             `json:"description,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Properties           map[string]*schema `json:"properties,omitempty"`
	AdditionalProperties bool               `json:"additionalProperties,omitempty"`
	Items                *schema            `json:"items,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`
	MaxLength            int                `json:"maxLength,omitempty"`
	MaxItems             int                `json:"maxItems,omitempty"`
	Format               string             `json:"format,omitempty"`
	// Source names a live source the viewer fills when the data omits the
	// property ("sprint": `bashy sprint show N --json`).
	Source string `json:"x-source,omitempty"`
}

type viewTemplate struct {
	Name   string
	Schema *schema
	Text   string
}

// loadTemplate finds NAME in the shaped dir first, then the shipped set.
func loadTemplate(dataDir, name string) (*viewTemplate, error) {
	if !kit.ValidName(name) {
		return nil, fmt.Errorf("template name %q: use 1-64 letters, digits, - or _", name)
	}
	read := func(file string) ([]byte, error) {
		if dataDir != "" {
			if raw, err := os.ReadFile(filepath.Join(dataDir, "templates", file)); err == nil {
				return raw, nil
			}
		}
		return shippedTemplates.ReadFile("templates/" + file)
	}
	text, err := read(name + ".md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("unknown template %q (have: %s)", name, strings.Join(templateNames(dataDir), ", "))
	}
	rawSchema, err := read(name + ".schema.json")
	if err != nil {
		return nil, fmt.Errorf("template %q has no %s.schema.json beside it", name, name)
	}
	s := &schema{}
	dec := json.NewDecoder(bytes.NewReader(rawSchema))
	dec.DisallowUnknownFields()
	if err := dec.Decode(s); err != nil {
		return nil, fmt.Errorf("template %q: invalid schema: %v", name, err)
	}
	if s.Type != "object" {
		return nil, fmt.Errorf("template %q: schema root must be an object", name)
	}
	return &viewTemplate{Name: name, Schema: s, Text: string(text)}, nil
}

// templateNames lists shipped plus shaped template names.
func templateNames(dataDir string) []string {
	seen := map[string]bool{}
	add := func(file string) {
		if n, ok := strings.CutSuffix(file, ".md.tmpl"); ok && kit.ValidName(n) {
			seen[n] = true
		}
	}
	if ents, err := shippedTemplates.ReadDir("templates"); err == nil {
		for _, e := range ents {
			add(e.Name())
		}
	}
	if ents, err := os.ReadDir(filepath.Join(dataDir, "templates")); err == nil {
		for _, e := range ents {
			add(e.Name())
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// validate checks v against s, returning the first violation by path.
func validate(s *schema, v any, path string) error {
	if s == nil {
		return nil
	}
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%s: %s", path, fmt.Sprintf(format, args...))
	}
	switch s.Type {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return bad("want an object, got %s", kind(v))
		}
		for _, r := range s.Required {
			if _, ok := m[r]; !ok {
				return fmt.Errorf("%s.%s: required", path, r)
			}
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			ps, ok := s.Properties[k]
			if !ok {
				if s.AdditionalProperties {
					continue
				}
				return fmt.Errorf("%s.%s: unknown field (allowed: %s)", path, k, strings.Join(propNames(s), ", "))
			}
			if err := validate(ps, m[k], path+"."+k); err != nil {
				return err
			}
		}
	case "array":
		a, ok := v.([]any)
		if !ok {
			return bad("want an array, got %s", kind(v))
		}
		if s.MaxItems > 0 && len(a) > s.MaxItems {
			return bad("at most %d items, got %d", s.MaxItems, len(a))
		}
		for i, e := range a {
			if err := validate(s.Items, e, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			return bad("want a string, got %s", kind(v))
		}
		max := s.MaxLength
		if max == 0 {
			max = 20000
		}
		if len(str) > max {
			return bad("at most %d characters, got %d", max, len(str))
		}
	case "number":
		if _, ok := v.(float64); !ok {
			return bad("want a number, got %s", kind(v))
		}
	case "integer":
		f, ok := v.(float64)
		if !ok || f != float64(int64(f)) {
			return bad("want an integer, got %s", kind(v))
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return bad("want true or false, got %s", kind(v))
		}
	case "":
	default:
		return bad("schema type %q is not supported", s.Type)
	}
	if len(s.Enum) > 0 {
		for _, e := range s.Enum {
			if e == v {
				return nil
			}
		}
		return bad("want one of %v, got %v", s.Enum, v)
	}
	return nil
}

func propNames(s *schema) []string {
	out := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func kind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "an object"
	case []any:
		return "an array"
	case string:
		return "a string"
	case float64:
		return "a number"
	case bool:
		return "a boolean"
	}
	return fmt.Sprintf("%T", v)
}

// mdText is a plain-text value: it prints with every ASCII punctuation
// character backslash-escaped and newlines folded, so it renders as itself.
type mdText string

func (t mdText) String() string { return escapeText(string(t)) }

// mdProse is a Markdown prose value: Markdown is honoured, raw HTML tags
// outside code are shown as text.
type mdProse string

func (p mdProse) String() string { return escapeProseHTML(string(p)) }

// breakMode "no-escape" disables value escaping so the red tests can prove
// the escaping tests detect its absence.
func escapeText(s string) string {
	if breakMode == "no-escape" {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\r':
		case r == '\n' || r == '\t':
			b.WriteByte(' ')
		case r < 128 && strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", r):
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeProseHTML turns "<" that would open a tag, comment or declaration
// into an entity, except inside code spans and fenced code, where Markdown
// already shows it literally.
func escapeProseHTML(s string) string {
	if breakMode == "no-escape" {
		return s
	}
	var out strings.Builder
	inFence := ""
	for i, ln := range strings.SplitAfter(s, "\n") {
		_ = i
		if m := fenceRe.FindStringSubmatch(ln); m != nil {
			if inFence == "" {
				inFence = m[1][:1]
			} else if m[1][:1] == inFence {
				inFence = ""
			}
			out.WriteString(ln)
			continue
		}
		if inFence != "" {
			out.WriteString(ln)
			continue
		}
		tick := 0 // length of the open code span's backtick run, 0 = none
		for j := 0; j < len(ln); j++ {
			c := ln[j]
			if c == '`' {
				k := j
				for k < len(ln) && ln[k] == '`' {
					k++
				}
				run := k - j
				if tick == 0 {
					tick = run
				} else if run == tick {
					tick = 0
				}
				out.WriteString(ln[j:k])
				j = k - 1
				continue
			}
			if c == '<' && tick == 0 && j+1 < len(ln) {
				n := ln[j+1]
				if n == '/' || n == '!' || n == '?' || (n|0x20 >= 'a' && n|0x20 <= 'z') {
					out.WriteString("&lt;")
					continue
				}
			}
			out.WriteByte(c)
		}
	}
	return out.String()
}

// wrap converts parsed JSON into template values per the schema.
func wrap(s *schema, v any) any {
	switch x := v.(type) {
	case string:
		if s != nil && s.Format == "markdown" {
			return mdProse(x)
		}
		return mdText(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			var ps *schema
			if s != nil {
				ps = s.Properties[k]
			}
			out[k] = wrap(ps, e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		var is *schema
		if s != nil {
			is = s.Items
		}
		for i, e := range x {
			out[i] = wrap(is, e)
		}
		return out
	}
	return v
}

var funcs = template.FuncMap{
	// json emits v as JSON safe inside a ```chart fence.
	"json": func(v any) (string, error) {
		b, err := json.Marshal(v) // escapes <, >, & as \u00xx
		if err != nil {
			return "", err
		}
		return strings.ReplaceAll(string(b), "`", "\\u0060"), nil
	},
	// code shows v as a code span whose fence outruns any backticks in it.
	"code": func(v any) string {
		s := strings.ReplaceAll(fmt.Sprint(rawString(v)), "\n", " ")
		fence := "`"
		for strings.Contains(s, fence) {
			fence += "`"
		}
		return fence + " " + s + " " + fence
	},
	// link renders a Markdown link to an http(s) URL; anything else is
	// shown as text.
	"link": func(label, target any) string {
		l, t := mdText(fmt.Sprint(rawString(label))), fmt.Sprint(rawString(target))
		u, err := url.Parse(t)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return l.String() + " (" + mdText(t).String() + ")"
		}
		return "[" + l.String() + "](<" + strings.NewReplacer("<", "%3C", ">", "%3E", " ", "%20", "\n", "").Replace(u.String()) + ">)"
	},
	"join": func(sep string, v any) string {
		a, _ := v.([]any)
		parts := make([]string, len(a))
		for i, e := range a {
			parts[i] = fmt.Sprint(e)
		}
		return strings.Join(parts, mdText(sep).String())
	},
	"default": func(def, v any) any {
		if v == nil || fmt.Sprint(rawString(v)) == "" {
			return def
		}
		return v
	},
	"add1": func(i int) int { return i + 1 },
	"check": func(v any) string {
		if b, _ := v.(bool); b {
			return "☑"
		}
		return "☐"
	},
	"count": func(field string, v any) int {
		a, _ := v.([]any)
		n := 0
		for _, e := range a {
			if m, ok := e.(map[string]any); ok && m[field] == true {
				n++
			}
		}
		return n
	},
}

func rawString(v any) any {
	switch x := v.(type) {
	case mdText:
		return string(x)
	case mdProse:
		return string(x)
	}
	return v
}

// sprintSource fetches the live card for the "sprint" x-source. It is a
// variable so tests run without a bashy board.
var sprintSource = func(n int) (map[string]any, error) {
	bin := os.Getenv("BASHY")
	if bin == "" {
		bin = "bashy"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "sprint", "show", fmt.Sprint(n), "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("bashy sprint show %d --json: %v", n, err)
	}
	return parseSprintGraph(out)
}

// parseSprintGraph reduces the `sprint show --json` graph (JSONL nodes and
// edges) to the card fields templates read.
func parseSprintGraph(raw []byte) (map[string]any, error) {
	card := map[string]any{}
	var goals []any
	stories := 0
	for _, ln := range bytes.Split(raw, []byte("\n")) {
		ln = bytes.TrimSpace(ln)
		if len(ln) == 0 {
			continue
		}
		var rec struct {
			T     string         `json:"t"`
			Kind  string         `json:"kind"`
			Attrs map[string]any `json:"attrs"`
		}
		if json.Unmarshal(ln, &rec) != nil || rec.T != "node" {
			continue
		}
		switch rec.Kind {
		case "sprint":
			for _, k := range []string{"id", "title", "column", "primary_goal", "acceptance", "continuity", "owner", "epic"} {
				if v, ok := rec.Attrs[k]; ok {
					card[k] = v
				}
			}
		case "goal":
			g := map[string]any{}
			for _, k := range []string{"id", "text", "checked", "wontfix"} {
				if v, ok := rec.Attrs[k]; ok {
					g[k] = v
				}
			}
			goals = append(goals, g)
		case "story":
			stories++
		}
	}
	if len(card) == 0 {
		return nil, errors.New("sprint card not found in sprint show output")
	}
	if goals == nil {
		goals = []any{}
	}
	card["goals"] = goals
	card["stories"] = float64(stories)
	return card, nil
}

// fillTemplate validates data, merges live sources, and renders Markdown.
func fillTemplate(dataDir, name string, data json.RawMessage, sprint int) (string, json.RawMessage, error) {
	t, err := loadTemplate(dataDir, name)
	if err != nil {
		return "", nil, err
	}
	var v any = map[string]any{}
	if len(bytes.TrimSpace(data)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&v); err != nil {
			return "", nil, fmt.Errorf("--data is not valid JSON: %v", err)
		}
		if dec.More() {
			return "", nil, errors.New("--data must be exactly one JSON object")
		}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return "", nil, fmt.Errorf("data: want an object, got %s", kind(v))
	}
	if secretKeys(m) {
		return "", nil, errors.New(kit.ErrSecret)
	}
	for _, k := range propNames(t.Schema) {
		ps := t.Schema.Properties[k]
		if ps.Source != "sprint" {
			continue
		}
		if _, given := m[k]; given {
			continue
		}
		if sprint <= 0 {
			if contains(t.Schema.Required, k) {
				return "", nil, fmt.Errorf("template %q reads the live sprint card: give --sprint N (or append to a sprint book)", name)
			}
			continue
		}
		card, err := sprintSource(sprint)
		if err != nil {
			return "", nil, err
		}
		m[k] = card
	}
	if breakMode != "no-validate" {
		if err := validate(t.Schema, m, "data"); err != nil {
			return "", nil, fmt.Errorf("data does not match template %q: %v", name, err)
		}
	}
	tmpl, err := template.New(name).Funcs(funcs).Option("missingkey=zero").Parse(t.Text)
	if err != nil {
		return "", nil, fmt.Errorf("template %q: %v", name, err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, wrap(t.Schema, m)); err != nil {
		return "", nil, fmt.Errorf("template %q: %v", name, err)
	}
	if out.Len() > 1<<20 {
		return "", nil, fmt.Errorf("template %q produced more than 1 MiB", name)
	}
	stored, _ := json.Marshal(m)
	return strings.ReplaceAll(out.String(), "<no value>", ""), stored, nil
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// secretKeys refuses data whose keys name a credential, the same rule
// desk fields follow.
func secretKeys(v any) bool {
	var fields []kit.Field
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				fields = append(fields, kit.Field{Name: k})
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	return kit.SecretRefused(fields)
}
