// Package main implements the desk form tool (F1): one engine for ad-hoc
// forms, replyforms (F2) and questionnaires (F3).
//
// This file holds the reusable core both follow-ups build on: the form
// definition schema, the answer store shape, validation, and a stdlib-only
// YAML-subset reader so agents can author definitions in JSON or YAML.
// The HTTP/CLI doors in main.go and cli.go are thin projections of it.
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	kit "desk-kit"
)

// Field kinds an agent may declare. "secret" is deliberately absent: a
// secret-typed or secret-named field is refused before storage and the
// refusal points at `bashy ask`/vault.
const (
	KindText     = "text"
	KindTextarea = "textarea"
	KindChoice   = "choice"
	KindMulti    = "multi"
	KindYesNo    = "yes-no"
	KindFile     = "file"
)

// FormField is one declared field. Recommended carries at most one option
// of a choice/multi field; the page renders it with a mark.
type FormField struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	Required    bool     `json:"required,omitempty"`
	Options     []string `json:"options,omitempty"`
	Recommended string   `json:"recommended,omitempty"`
	Help        string   `json:"help,omitempty"`
}

// Definition is the ordered field list of a form. Questionnaire (F3)
// follow-up rounds append fields; replyform (F2) builds one from a browser
// extract. Answers below stay keyed by field id across all three uses.
type Definition []FormField

// Answers maps field id to its answer: a string for text, textarea,
// choice, yes-no ("yes"/"no") and file (file name, or
// {"name":..,"size":..}), a string array for multi.
type Answers map[string]any

// fieldIDs reports duplicate or ill-formed field ids.
func checkIDs(def Definition) error {
	seen := map[string]bool{}
	for _, f := range def {
		if !kit.ValidName(f.ID) {
			return fmt.Errorf("field %q: use 1-64 letters, digits, - or _ for an id", f.ID)
		}
		if seen[f.ID] {
			return fmt.Errorf("field %q: declared twice", f.ID)
		}
		seen[f.ID] = true
	}
	return nil
}

// ValidateDefinition rejects secret smuggling, unknown kinds, option
// misuse and dangling recommended marks before anything is stored.
func ValidateDefinition(def Definition) error {
	if err := checkIDs(def); err != nil {
		return err
	}
	for _, f := range def {
		// Secrets never flow through desk: reuse the shared refusal so a
		// secret kind or a secret-like id (db_password, api-key, …) is
		// refused exactly like a secret-typed generic field.
		if kit.SecretRefused([]kit.Field{{Name: f.ID, Type: f.Kind}}) {
			return fmt.Errorf("%s (field %q)", kit.ErrSecret, f.ID)
		}
		if strings.TrimSpace(f.Label) == "" {
			return fmt.Errorf("field %q: a label is required", f.ID)
		}
		switch f.Kind {
		case KindText, KindTextarea, KindYesNo, KindFile:
			if len(f.Options) > 0 || f.Recommended != "" {
				return fmt.Errorf("field %q: kind %q takes no options or recommended mark", f.ID, f.Kind)
			}
		case KindChoice, KindMulti:
			if len(f.Options) == 0 {
				return fmt.Errorf("field %q: kind %q needs at least one option", f.ID, f.Kind)
			}
			if f.Recommended != "" && !contains(f.Options, f.Recommended) {
				return fmt.Errorf("field %q: recommended %q is not one of its options", f.ID, f.Recommended)
			}
		default:
			return fmt.Errorf("field %q: unknown kind %q (want text, textarea, choice, multi, yes-no or file)", f.ID, f.Kind)
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// byID indexes a definition for answer checks.
func (d Definition) byID() map[string]FormField {
	m := make(map[string]FormField, len(d))
	for _, f := range d {
		m[f.ID] = f
	}
	return m
}

// emptyAnswer reports a value that does not count as filled.
func emptyAnswer(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case []string:
		return len(t) == 0
	case map[string]any:
		name, _ := t["name"].(string)
		return strings.TrimSpace(name) == ""
	}
	return false
}

// NormalizeAnswers checks answer shapes against the definition, normalises
// yes-no spellings to "yes"/"no", and — when complete is true — enforces
// required fields and choice membership. Partial saves (complete false)
// store whatever the human has so far; unknown field ids are always an
// error so a misspelled id cannot silently vanish.
func NormalizeAnswers(def Definition, ans Answers, complete bool) (Answers, error) {
	// Secret-named answer keys are refused like secret fields: credentials
	// stay with `bashy ask`/vault, never in a desk answer store.
	for k := range ans {
		if kit.SecretRefused([]kit.Field{{Name: k}}) {
			return nil, fmt.Errorf("%s (answer %q)", kit.ErrSecret, k)
		}
	}
	byID := def.byID()
	out := make(Answers, len(ans))
	for id, v := range ans {
		f, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("answer %q: no such field in this form", id)
		}
		nv, err := normalizeOne(f, v)
		if err != nil {
			return nil, err
		}
		out[id] = nv
	}
	if !complete {
		return out, nil
	}
	var missing []string
	for _, f := range def {
		if f.Required && emptyAnswer(out[f.ID]) {
			missing = append(missing, f.ID)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	for id, v := range out {
		if err := checkMembership(byID[id], v); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// normalizeOne coerces one answer to its stored shape.
func normalizeOne(f FormField, v any) (any, error) {
	switch f.Kind {
	case KindMulti:
		arr, ok := toStringSlice(v)
		if !ok {
			return nil, fmt.Errorf("answer %q: want an array of strings", f.ID)
		}
		seen := map[string]bool{}
		dedup := []string{}
		for _, s := range arr {
			if !seen[s] {
				seen[s] = true
				dedup = append(dedup, s)
			}
		}
		if dedup == nil {
			dedup = []string{}
		}
		return dedup, nil
	case KindYesNo:
		switch t := v.(type) {
		case bool:
			if t {
				return "yes", nil
			}
			return "no", nil
		case string:
			switch strings.ToLower(strings.TrimSpace(t)) {
			case "yes", "y", "true", "1":
				return "yes", nil
			case "no", "n", "false", "0", "":
				if strings.TrimSpace(t) == "" {
					return "", nil
				}
				return "no", nil
			}
		}
		return nil, fmt.Errorf("answer %q: want yes or no", f.ID)
	case KindFile:
		switch t := v.(type) {
		case string:
			return t, nil
		case map[string]any:
			return t, nil
		}
		return nil, fmt.Errorf("answer %q: want a file name or {name,size}", f.ID)
	default: // text, textarea, choice
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("answer %q: want a string", f.ID)
		}
		return s, nil
	}
}

// checkMembership enforces option membership on complete submits.
func checkMembership(f FormField, v any) error {
	switch f.Kind {
	case KindChoice:
		if s, _ := v.(string); s != "" && !contains(f.Options, s) {
			return fmt.Errorf("answer %q: %q is not one of its options", f.ID, s)
		}
	case KindMulti:
		if arr, ok := v.([]string); ok {
			for _, s := range arr {
				if !contains(f.Options, s) {
					return fmt.Errorf("answer %q: %q is not one of its options", f.ID, s)
				}
			}
		}
	}
	return nil
}

func toStringSlice(v any) ([]string, bool) {
	switch t := v.(type) {
	case nil:
		return []string{}, true
	case []string:
		return t, true
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	case string:
		if t == "" {
			return []string{}, true
		}
		return []string{t}, true
	}
	return nil, false
}

// ParseDefinition reads a form definition in JSON or YAML. A .yaml/.yml
// file (or YAML-looking text) goes through the stdlib-only YAML subset
// below; otherwise the input must be JSON. Both shapes accept either a bare
// field array or {"title":.., "fields":[..]}.
func ParseDefinition(data []byte, filename string) (title string, def Definition, err error) {
	title, def, _, err = ParseFormDefinition(data, filename)
	return
}

// ParseFormDefinition includes the respondent mode declared beside fields.
func ParseFormDefinition(data []byte, filename string) (title string, def Definition, respondents string, err error) {
	if strings.HasSuffix(strings.ToLower(filename), ".yaml") ||
		strings.HasSuffix(strings.ToLower(filename), ".yml") ||
		looksLikeYAML(data) {
		data, err = yamlSubsetToJSON(data)
		if err != nil {
			return "", nil, "", err
		}
	}
	var bare []FormField
	if err := json.Unmarshal(data, &bare); err == nil {
		if err := ValidateDefinition(bare); err != nil {
			return "", nil, "", err
		}
		return "", bare, "one", nil
	}
	var doc struct {
		Title       string     `json:"title"`
		Fields      Definition `json:"fields"`
		Respondents string     `json:"respondents"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return "", nil, "", fmt.Errorf("invalid form definition: %v", err)
	}
	if doc.Fields == nil {
		return "", nil, "", fmt.Errorf("invalid form definition: want a field array or {\"title\", \"fields\"}")
	}
	if err := ValidateDefinition(doc.Fields); err != nil {
		return "", nil, "", err
	}
	if doc.Respondents == "" {
		doc.Respondents = "one"
	}
	if doc.Respondents != "one" && doc.Respondents != "many" {
		return "", nil, "", fmt.Errorf("respondents must be one or many")
	}
	return doc.Title, doc.Fields, doc.Respondents, nil
}

func looksLikeYAML(data []byte) bool {
	s := strings.TrimSpace(string(data))
	if s == "" || strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return false
	}
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "-	") {
			return true
		}
		if k := strings.Index(t, ":"); k > 0 && !strings.Contains(t[:k], " ") {
			rest := t[k+1:]
			if rest == "" || strings.HasPrefix(rest, " ") {
				return true
			}
		}
	}
	return false
}

// --- stdlib-only YAML subset ---
//
// Enough for agent-authored form definitions: block mappings, lists of
// scalars or mappings, nested blocks, inline [a, b] lists, quoted scalars,
// true/false booleans and # comments. Anything fancier (anchors, multi-line
// scalars, tabs) is rejected with a line number so the agent can fix it.

type yamlLine struct {
	n      int
	indent int
	text   string
}

func splitYAMLLines(data []byte) ([]yamlLine, error) {
	var out []yamlLine
	for i, raw := range strings.Split(string(data), "\n") {
		if strings.Contains(raw, "\t") {
			// Tabs are indentation errors in YAML; fail loudly by line.
			trimmed := strings.TrimLeft(raw, " \t")
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				return nil, fmt.Errorf("invalid YAML line %d: tabs are not indentation", i+1)
			}
		}
		n := 0
		for n < len(raw) && raw[n] == ' ' {
			n++
		}
		text := stripYAMLComment(raw[n:])
		text = strings.TrimRight(text, " ")
		if text == "" || text == "---" || text == "..." {
			continue
		}
		out = append(out, yamlLine{n: i + 1, indent: n, text: text})
	}
	return out, nil
}

func stripYAMLComment(s string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
				return s[:i-1]
			}
		}
	}
	return s
}

func parseYAMLScalar(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var v string
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			return nil, fmt.Errorf("bad double-quoted string %q", s)
		}
		return v, nil
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'"), nil
	}
	switch s {
	case "true", "True", "TRUE":
		return true, nil
	case "false", "False", "FALSE":
		return false, nil
	case "null", "~", "Null", "NULL":
		return nil, nil
	}
	if strings.HasPrefix(s, "[") {
		if !strings.HasSuffix(s, "]") {
			return nil, fmt.Errorf("bad inline list %q", s)
		}
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == "" {
			return []any{}, nil
		}
		var out []any
		for _, part := range splitInline(inner) {
			v, err := parseYAMLScalar(part)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	return s, nil
}

func splitInline(s string) []string {
	var parts []string
	depth := 0
	inSingle, inDouble := false, false
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '[':
			if !inSingle && !inDouble {
				depth++
			}
		case ']':
			if !inSingle && !inDouble {
				depth--
			}
		case ',':
			if !inSingle && !inDouble && depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(parts, strings.TrimSpace(s[start:]))
}

type yamlParser struct {
	lines []yamlLine
	pos   int
}

func yamlSubsetToJSON(data []byte) ([]byte, error) {
	lines, err := splitYAMLLines(data)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("invalid YAML: empty definition")
	}
	p := &yamlParser{lines: lines}
	v, err := p.parseBlock(0)
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.lines) {
		return nil, fmt.Errorf("invalid YAML line %d: unexpected %q", p.lines[p.pos].n, p.lines[p.pos].text)
	}
	return json.Marshal(v)
}

func (p *yamlParser) parseBlock(minIndent int) (any, error) {
	if p.pos >= len(p.lines) {
		return nil, fmt.Errorf("invalid YAML: unexpected end of input")
	}
	if strings.HasPrefix(p.lines[p.pos].text, "- ") || p.lines[p.pos].text == "-" {
		return p.parseList(p.lines[p.pos].indent)
	}
	return p.parseMap(minIndent)
}

func (p *yamlParser) parseMap(minIndent int) (map[string]any, error) {
	m := map[string]any{}
	for p.pos < len(p.lines) {
		ln := p.lines[p.pos]
		if ln.indent < minIndent || strings.HasPrefix(ln.text, "- ") || ln.text == "-" {
			break
		}
		if ln.indent > minIndent {
			return nil, fmt.Errorf("invalid YAML line %d: bad indentation", ln.n)
		}
		key, rest, ok := splitMapKey(ln.text)
		if !ok {
			return nil, fmt.Errorf("invalid YAML line %d: want key: value, got %q", ln.n, ln.text)
		}
		p.pos++
		if rest != "" {
			v, err := parseYAMLScalar(rest)
			if err != nil {
				return nil, fmt.Errorf("invalid YAML line %d: %v", ln.n, err)
			}
			m[key] = v
			continue
		}
		if p.pos >= len(p.lines) || p.lines[p.pos].indent <= ln.indent {
			m[key] = ""
			continue
		}
		v, err := p.parseBlock(ln.indent + 1)
		if err != nil {
			return nil, err
		}
		m[key] = v
	}
	return m, nil
}

func (p *yamlParser) parseList(minIndent int) ([]any, error) {
	var out []any
	for p.pos < len(p.lines) {
		ln := p.lines[p.pos]
		if ln.indent < minIndent || (!strings.HasPrefix(ln.text, "- ") && ln.text != "-") {
			break
		}
		if ln.indent > minIndent {
			return nil, fmt.Errorf("invalid YAML line %d: bad indentation", ln.n)
		}
		rest := strings.TrimSpace(strings.TrimPrefix(ln.text, "-"))
		p.pos++
		if rest == "" {
			if p.pos >= len(p.lines) || p.lines[p.pos].indent <= ln.indent {
				out = append(out, "")
				continue
			}
			v, err := p.parseBlock(ln.indent + 1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		if key, val, ok := splitMapKey(rest); ok {
			m := map[string]any{}
			if val != "" {
				v, err := parseYAMLScalar(val)
				if err != nil {
					return nil, fmt.Errorf("invalid YAML line %d: %v", ln.n, err)
				}
				m[key] = v
			} else if p.pos < len(p.lines) && p.lines[p.pos].indent > ln.indent {
				v, err := p.parseBlock(ln.indent + 1)
				if err != nil {
					return nil, err
				}
				m[key] = v
			} else {
				m[key] = ""
			}
			// Continuation keys of the same list item share the item indent.
			for p.pos < len(p.lines) && p.lines[p.pos].indent > ln.indent &&
				!strings.HasPrefix(p.lines[p.pos].text, "- ") && p.lines[p.pos].text != "-" {
				cln := p.lines[p.pos]
				ck, cv, ok := splitMapKey(cln.text)
				if !ok {
					return nil, fmt.Errorf("invalid YAML line %d: want key: value, got %q", cln.n, cln.text)
				}
				p.pos++
				if cv != "" {
					v, err := parseYAMLScalar(cv)
					if err != nil {
						return nil, fmt.Errorf("invalid YAML line %d: %v", cln.n, err)
					}
					m[ck] = v
					continue
				}
				if p.pos < len(p.lines) && p.lines[p.pos].indent > cln.indent {
					v, err := p.parseBlock(cln.indent + 1)
					if err != nil {
						return nil, err
					}
					m[ck] = v
				} else {
					m[ck] = ""
				}
			}
			out = append(out, m)
			continue
		}
		v, err := parseYAMLScalar(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid YAML line %d: %v", ln.n, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func splitMapKey(s string) (key, rest string, ok bool) {
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case ':':
			if !inSingle && !inDouble && (i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\t') {
				key = strings.TrimSpace(s[:i])
				if key == "" {
					return "", "", false
				}
				return key, strings.TrimSpace(s[i+1:]), true
			}
		}
	}
	return "", "", false
}
