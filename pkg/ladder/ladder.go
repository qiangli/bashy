// Package ladder resolves one input line on the Sprint 255 action ladder
// (docs/bashy-agentic-chat-mode-design.md §2): a literal command runs as
// typed (rung 0), a misspelled command name is repaired deterministically
// (rung 1), and anything else is free text, which the caller hands to an
// agent as a turn (the last rung). No rung here calls a model.
//
// Rung 0 is strict: a line whose first word is a command the shell can run is
// never reinterpreted, so a shell that routes its input through Resolve stays
// a superset of the classic shell. The one exception is a line with a word
// ending in a question mark that matches no file ("what does this do?"): an
// unmatched glob means nothing to the shell, so it is read as a question. Recall (rung 2) is not implemented yet.
package ladder

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"mvdan.cc/sh/v3/syntax"
)

// Rung is the step that resolved a line.
type Rung int

const (
	Literal Rung = iota // run as typed
	Repair              // run the repaired line; echo it first
	Free                // not a command: an agent turn
)

func (r Rung) String() string {
	switch r {
	case Literal:
		return "literal"
	case Repair:
		return "repair"
	}
	return "free"
}

// Commands is what the ladder knows about the shell it routes for.
type Commands interface {
	// Known reports whether name runs as a command: a keyword, builtin,
	// function, alias, owned command or an executable on PATH.
	Known(name string) bool
	// Names lists the command names a typo may be repaired to.
	Names() []string
}

// Globber is optional on Commands: it reports whether a glob pattern matches
// a file, so a trailing "?" can be told apart from a pattern. Without it,
// a trailing "?" is always a question.
type Globber interface {
	GlobMatches(pattern string) bool
}

// Decision is the resolution of one line.
type Decision struct {
	Rung Rung
	Line string // the line to run (Literal, Repair) or the turn text (Free)
	Note string // Repair: what was changed, for the echo
}

// Resolve places line on the ladder.
func Resolve(line string, cmds Commands) Decision {
	text := strings.TrimSpace(line)
	word, rest := firstWord(text)
	if word == "" || shellSyntax(word) || cmds.Known(word) && !question(text, cmds) {
		return Decision{Rung: Literal, Line: line}
	}
	if fixed, ok := repair(word, text, cmds.Names()); ok {
		return Decision{Rung: Repair, Line: fixed + rest, Note: word + " → " + fixed}
	}
	return Decision{Rung: Free, Line: text}
}

// question reports a line that asks rather than commands: a word after the
// command word ends in an unquoted "?" and globs no file. "what does this
// repo do?" and "which file has the bug? one sentence" are questions even
// though what(1) and which(1) exist; "ls file?" with file1 present stays a
// command. The shell passes an unmatched glob on as literal text, so no
// valid pattern loses its meaning.
func question(text string, cmds Commands) bool {
	fields := strings.Fields(text)
	for _, word := range fields[1:] {
		word = strings.TrimRight(word, ".,;:!)")
		if !strings.HasSuffix(word, "?") || strings.HasSuffix(word, "\\?") || strings.ContainsAny(word, "\"'`$*[") {
			continue
		}
		if g, ok := cmds.(Globber); ok && g.GlobMatches(word) {
			continue
		}
		return true
	}
	return false
}

// firstWord splits off the command word: up to the first blank or shell
// operator character.
func firstWord(text string) (word, rest string) {
	end := strings.IndexFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(";|&<>()", r)
	})
	if end < 0 {
		return text, ""
	}
	return text[:end], text[end:]
}

// shellSyntax reports a first word that is shell grammar rather than a
// command name: an empty first word (a leading operator), a path, an
// assignment, an expansion, a quote, a comment, a grouping or a keyword.
func shellSyntax(word string) bool {
	if word == "" || syntax.IsKeyword(word) {
		return true
	}
	switch word[0] {
	case '(', '{', '!', '[', '.', ':', '#', '$', '~', '/', '"', '\'', '`', '\\', '<', '>':
		return true
	}
	if strings.Contains(word, "/") {
		return true
	}
	if i := strings.IndexByte(word, '='); i > 0 && validName(word[:strings.IndexAny(word, "+=[")]) {
		return true
	}
	return false
}

func validName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

// repair finds the one command name the first word misspells: one edit away
// (an adjacent transposition counts as one) for words of three or more
// characters, two for seven or more. A tie, a question, or a word that is
// no command-like token leaves the line to the agent.
func repair(word, text string, names []string) (string, bool) {
	n := utf8.RuneCountInString(word)
	if n < 3 || strings.HasSuffix(text, "?") || !commandLike(word) {
		return "", false
	}
	limit := 1
	if n >= 7 {
		limit = 2
	}
	best, count, bestDist := "", 0, limit+1
	for _, name := range names {
		if name == word || abs(utf8.RuneCountInString(name)-n) > limit {
			continue
		}
		d := distance(word, name)
		switch {
		case d < bestDist:
			best, count, bestDist = name, 1, d
		case d == bestDist && name != best:
			count++
		}
	}
	if count != 1 || bestDist > limit {
		return "", false
	}
	return best, true
}

func commandLike(word string) bool {
	for _, r := range word {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '.' && r != '+' {
			return false
		}
	}
	return true
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// distance is the optimal-string-alignment edit distance: insertions,
// deletions, substitutions and adjacent transpositions each cost one.
func distance(a, b string) int {
	s, t := []rune(a), []rune(b)
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && s[i-1] == t[j-2] && s[i-2] == t[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(s)][len(t)]
}
