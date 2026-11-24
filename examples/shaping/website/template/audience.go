package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
)

// Shaping scope names in shaped.json (matching R5a appserver.Scopes).
// "view" is the existing cloudbox share itself and is not stored here.
const (
	ScopeEdit  = "edit"
	ScopeShape = "shape"
)

// ScopeAudience is the member-only audience grammar used inside shaped.json's
// scopes: circles AND (groups OR emails). An empty audience admits nobody.
type ScopeAudience struct {
	Circles []string `json:"circles,omitempty"`
	Groups  []string `json:"groups,omitempty"`
	Emails  []string `json:"emails,omitempty"`
}

// Empty reports whether the scope audience names nothing.
func (a *ScopeAudience) Empty() bool {
	return a == nil || (len(a.Circles) == 0 && len(a.Groups) == 0 && len(a.Emails) == 0)
}

// AdmitsMember reports whether a verified member stamp matches this scope
// audience: circles AND (groups OR emails). Fail-closed when empty or non-member.
func (a *ScopeAudience) AdmitsMember(c Caller) bool {
	if c.Tier != TierMember || a.Empty() {
		return false
	}
	circle := strings.TrimSpace(c.Circle)
	email := strings.TrimSpace(c.Email)
	if len(a.Circles) > 0 && !slices.Contains(a.Circles, circle) {
		return false
	}
	if len(a.Groups) == 0 && len(a.Emails) == 0 {
		return true
	}
	for _, g := range c.Groups {
		if g = strings.TrimSpace(g); g != "" && slices.Contains(a.Groups, g) {
			return true
		}
	}
	if email == "" {
		return false
	}
	return slices.ContainsFunc(a.Emails, func(e string) bool {
		return strings.EqualFold(strings.TrimSpace(e), email)
	})
}

// Scopes maps recognized shaping scopes ("edit", "shape") to member audiences.
type Scopes struct {
	Edit  *ScopeAudience `json:"edit,omitempty"`
	Shape *ScopeAudience `json:"shape,omitempty"`
}

// ScopeFor returns the caller's effective shaping scope ("shape", "edit", or ""):
// the owner always holds "shape"; a verified member gets the highest scope their
// stamp matches ("shape" includes "edit"); account and anonymous get "".
func (s Scopes) ScopeFor(c Caller) string {
	switch c.Tier {
	case TierOwner:
		return ScopeShape
	case TierMember:
		if s.Shape.AdmitsMember(c) {
			return ScopeShape
		}
		if s.Edit.AdmitsMember(c) {
			return ScopeEdit
		}
		return ""
	default:
		return ""
	}
}

type shapedManifest struct {
	Server     json.RawMessage `json:"server,omitempty"`
	Repair     json.RawMessage `json:"repair,omitempty"`
	Tidy       json.RawMessage `json:"tidy,omitempty"`
	Properties json.RawMessage `json:"properties,omitempty"`
	Genie      json.RawMessage `json:"genie,omitempty"`
	Scopes     *Scopes         `json:"scopes,omitempty"`
}

// parseShapedScopes decodes and validates the "scopes" section of shaped.json,
// refusing unknown scope names and malformed entries fail-closed.
func parseShapedScopes(raw []byte) (Scopes, error) {
	var m shapedManifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Scopes{}, fmt.Errorf("shaped.json: %w", err)
	}
	if dec.More() {
		return Scopes{}, errors.New("shaped.json: trailing data after manifest")
	}
	if m.Scopes == nil {
		return Scopes{}, nil
	}
	if err := cleanScopeAudience(ScopeEdit, m.Scopes.Edit); err != nil {
		return Scopes{}, err
	}
	if err := cleanScopeAudience(ScopeShape, m.Scopes.Shape); err != nil {
		return Scopes{}, err
	}
	return *m.Scopes, nil
}

func cleanScopeAudience(scope string, a *ScopeAudience) error {
	if a == nil {
		return nil
	}
	circles := make([]string, 0, len(a.Circles))
	for i, c := range a.Circles {
		c = strings.TrimSpace(c)
		if c == "" || strings.ContainsAny(c, " \t\r\n,<>") {
			return fmt.Errorf("shaped.json: scopes.%s.circles[%d] must be a non-empty circle name", scope, i)
		}
		if !slices.Contains(circles, c) {
			circles = append(circles, c)
		}
	}
	a.Circles = circles
	groups := make([]string, 0, len(a.Groups))
	for i, g := range a.Groups {
		g = strings.TrimSpace(g)
		if g == "" || strings.ContainsAny(g, ",\r\n<>") {
			return fmt.Errorf("shaped.json: scopes.%s.groups[%d] must be a non-empty group name", scope, i)
		}
		if !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	a.Groups = groups
	emails := make([]string, 0, len(a.Emails))
	for i, e := range a.Emails {
		e = strings.ToLower(strings.TrimSpace(e))
		local, domain, ok := strings.Cut(e, "@")
		if !ok || local == "" || domain == "" || strings.ContainsAny(e, " \t\r\n,<>") {
			return fmt.Errorf("shaped.json: scopes.%s.emails[%d] %q must be a valid email address", scope, i, e)
		}
		if !slices.Contains(emails, e) {
			emails = append(emails, e)
		}
	}
	a.Emails = emails
	return nil
}

func loadScopes(fsys fs.FS) (Scopes, error) {
	raw, err := fs.ReadFile(fsys, "shaped.json")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Scopes{}, nil
		}
		return Scopes{}, err
	}
	return parseShapedScopes(raw)
}

// Audience is the cloudbox rule grammar plus account: circles AND (group OR
// email) for members, or any logged-in cloudbox account. The legacy config key
// signed_in is kept as an alias of account. An empty audience is owner only —
// fail closed.
type Audience struct {
	Circles  []string `json:"circles,omitempty"`
	Groups   []string `json:"groups,omitempty"`
	Emails   []string `json:"emails,omitempty"`
	Account  bool     `json:"account,omitempty"`
	SignedIn bool     `json:"signed_in,omitempty"`
}

func (a Audience) admitsAccount() bool {
	return a.Account || a.SignedIn
}

func (a Audience) empty() bool {
	return !a.admitsAccount() && len(a.Circles) == 0 && len(a.Groups) == 0 && len(a.Emails) == 0
}

// Admits reports whether c may see something with this audience.
func (a Audience) Admits(c Caller) bool {
	switch c.Tier {
	case TierOwner:
		return true
	case TierAnonymous, "":
		return false
	}
	if a.admitsAccount() {
		return true
	}
	if c.Tier != TierMember || a.empty() {
		return false
	}
	if len(a.Circles) > 0 && !slices.Contains(a.Circles, c.Circle) {
		return false
	}
	if len(a.Groups) == 0 && len(a.Emails) == 0 {
		return true
	}
	for _, g := range c.Groups {
		if slices.Contains(a.Groups, g) {
			return true
		}
	}
	return slices.ContainsFunc(a.Emails, func(e string) bool { return strings.EqualFold(e, c.Email) })
}

// Field is one private fact the site can show to an audience.
type Field struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Body     string   `json:"body"`
	Audience Audience `json:"-"`
}

// loadFields reads fields/*.md. Each file has a flat front matter block:
//
//	---
//	label: Contact
//	circles: recruiter
//	account: false
//	---
//	body text
//
// Lists are comma-separated. The legacy key signed_in aliases account.
// Unknown keys are ignored.
func loadFields(fsys fs.FS, dir string) ([]Field, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, nil
	}
	var out []Field
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		f := Field{Name: strings.TrimSuffix(e.Name(), ".md")}
		body := string(raw)
		if strings.HasPrefix(body, "---\n") {
			if end := strings.Index(body[4:], "\n---"); end >= 0 {
				parseFront(body[4:4+end], &f)
				body = strings.TrimLeft(body[4+end+4:], "\n")
			}
		}
		f.Body = strings.TrimSpace(body)
		if f.Label == "" {
			f.Label = f.Name
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func parseFront(front string, f *Field) {
	sc := bufio.NewScanner(strings.NewReader(front))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "label":
			f.Label = v
		case "circles":
			f.Audience.Circles = list(v)
		case "groups":
			f.Audience.Groups = list(v)
		case "emails":
			f.Audience.Emails = list(v)
		case "account":
			f.Audience.Account = v == "true"
		case "signed_in":
			f.Audience.SignedIn = v == "true"
		}
	}
}

func list(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
