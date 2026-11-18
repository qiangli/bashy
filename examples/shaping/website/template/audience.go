package main

import (
	"bufio"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
)

// Audience is the cloudbox rule grammar plus signed_in: circles AND (group OR
// email) for members, or any signed-in account. An empty audience is owner
// only — fail closed.
type Audience struct {
	Circles  []string `json:"circles,omitempty"`
	Groups   []string `json:"groups,omitempty"`
	Emails   []string `json:"emails,omitempty"`
	SignedIn bool     `json:"signed_in,omitempty"`
}

func (a Audience) empty() bool {
	return !a.SignedIn && len(a.Circles) == 0 && len(a.Groups) == 0 && len(a.Emails) == 0
}

// Admits reports whether c may see something with this audience.
func (a Audience) Admits(c Caller) bool {
	switch c.Tier {
	case TierOwner:
		return true
	case TierAnonymous, "":
		return false
	}
	if a.SignedIn {
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
//	signed_in: false
//	---
//	body text
//
// Lists are comma-separated. Unknown keys are ignored.
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
