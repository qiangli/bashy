// Package skills embeds the tier-2 workspace skill shipped with the bashy
// binary, so it resolves from any cwd WITHOUT the source tree present — the
// skill content is compiled into `bashy`, not read from this repo. Surface it
// with `bashy skill` (see internal/agentos).
//
// `bashy` is the ONLY installed skill: every bashy command is a skill too
// (`bashy <cmd> --help` renders it; see internal/agentos/skillhelp.go), so a
// second top-level skill folder would just be a second way to say the same
// thing. A front-door command that wants a long-form doc beyond its own
// generated help adds one under bashy/commands/<cmd>.md (an optional
// bashy/reference/<cmd>.md companion, and bashy/commands/<cmd>.dhnt for a
// machine-checkable contract) — still embedded through this one folder, and
// still reachable as `bashy skill show/run <cmd>` via CommandDoc/CommandNames
// below bridging those names into the skill catalog (internal/agentos).
package skills

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed all:bashy
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

// CommandNames returns the name of every front-door command with a moved doc
// under bashy/commands (those with a <cmd>.md), sorted.
func CommandNames() []string {
	entries, err := fs.ReadDir(FS, "bashy/commands")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(names)
	return names
}

// CommandDoc returns the embedded long-form doc for a front-door command
// moved into the bashy skill folder (bashy/commands/<name>.md) — the full
// original skill body, frontmatter included — or ("", false) if the command
// has none.
func CommandDoc(name string) (string, bool) { return readBashy("commands", name, ".md") }

// CommandReference returns the embedded reference.md companion for a command
// doc (bashy/reference/<name>.md), if it has one.
func CommandReference(name string) (string, bool) { return readBashy("reference", name, ".md") }

// CommandDhnt returns the embedded skill.dhnt companion for a command doc
// (bashy/commands/<name>.dhnt), if it declares a machine-checkable contract.
func CommandDhnt(name string) (string, bool) { return readBashy("commands", name, ".dhnt") }

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

// readBashy reads bashy/<dir>/<name><ext>, rejecting a name that is not a
// single path segment.
func readBashy(dir, name, ext string) (string, bool) {
	name = strings.Trim(name, "/")
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	data, err := fs.ReadFile(FS, "bashy/"+dir+"/"+name+ext)
	if err != nil {
		return "", false
	}
	return string(data), true
}
