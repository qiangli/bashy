// Package skills embeds the tier-2 workspace skills shipped with the bashy
// binary, so a skill resolves from any cwd WITHOUT the source tree present —
// the skill content is compiled into `bashy`, not read from this repo. Surface
// them with `bashy skills` (see internal/agentos).
//
// Each skill is a directory holding a `SKILL.md` (the actionable checklist,
// required) and an optional `reference.md` (deep companion). Add a new skill by
// dropping its directory here and adding it to the //go:embed directive below.
package skills

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed all:conductor all:go-repo-health all:bashy
var FS embed.FS

// Names returns every embedded skill directory name (those with a SKILL.md),
// sorted.
func Names() []string {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := fs.Stat(FS, e.Name()+"/SKILL.md"); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// Body returns the embedded SKILL.md for a skill, or ("", false) if absent.
func Body(name string) (string, bool) { return read(name, "SKILL.md") }

// Reference returns the embedded reference.md for a skill, if it has one.
func Reference(name string) (string, bool) { return read(name, "reference.md") }

func read(name, file string) (string, bool) {
	name = strings.Trim(name, "/")
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	data, err := fs.ReadFile(FS, name+"/"+file)
	if err != nil {
		return "", false
	}
	return string(data), true
}
