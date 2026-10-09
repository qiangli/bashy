// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package corpuslocales

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Staging rules for the provisioned corpus locales. See corpuslocales.go for
// the provenance record and the rationale of each rule. All functions are
// byte-preserving: they match ASCII patterns only and never recode text.

// stageLocaleSource applies the source staging rules for one locale file.
func stageLocaleSource(name string, data []byte) []byte {
	lines := splitStagedLines(data)
	// Rule 1: drop LC_COLLATE everywhere (multi-script ISO 14651 orders
	// exceed the certified compiler; refused by name instead of faked).
	lines = dropStagedSection(lines, "LC_COLLATE")
	switch name {
	case "i18n_ctype":
		// Rule 3: custom class/map blocks the store cannot carry.
		for _, prefix := range []string{`map "totitle"`, `class "combining"`, `class "combining_level3"`} {
			lines = dropStagedBlock(lines, prefix)
		}
		// Rule 4: the compiler's ellipsis forms cover single-byte ASCII
		// only; enumerate ranges exactly.
		lines = expandStagedURanges(lines)
	case "zh_CN":
		// Rule 3: the hanzi class (tab-separated keyword, like glibc).
		lines = dropStagedBlock(lines, "class")
		lines = dropCopyOnlyStagedSection(lines, "LC_PAPER")
		lines = dropCopyOnlyStagedSection(lines, "LC_MEASUREMENT")
	case "ja_JP":
		// Rule 3: kana classes, kana maps and their declarations.
		for _, prefix := range []string{"charclass", "charconv", "jspace", "jdigit", "jhira", "jkata", "jkanji", "tojhira", "tojkata"} {
			lines = dropStagedBlock(lines, prefix)
		}
		lines = dropCopyOnlyStagedSection(lines, "LC_PAPER")
		lines = dropCopyOnlyStagedSection(lines, "LC_MEASUREMENT")
	case "en_US":
		// Real LC_PAPER/LC_MEASUREMENT sections skip silently; keep them.
	default:
		// Rule 2: pure-copy extension sections can never resolve.
		lines = dropCopyOnlyStagedSection(lines, "LC_PAPER")
		lines = dropCopyOnlyStagedSection(lines, "LC_MEASUREMENT")
	}
	// Rule 5: pre-join continuations glibc joins before comment stripping.
	lines = joinStagedCommentContinuations(lines)
	return joinStagedLines(lines)
}

// stageCharmap applies the charmap staging rules. Only the UTF-8 charmap
// needs rewriting (rule 6); every other charmap ships as fetched.
func stageCharmap(name string, data []byte) []byte {
	if name != "UTF-8" {
		return data
	}
	return expandStagedCharmapRanges(data)
}

// splitStagedLines splits on \n keeping every byte intact (no charset
// conversion anywhere in staging).
func splitStagedLines(data []byte) []string {
	return strings.Split(string(data), "\n")
}

func joinStagedLines(lines []string) []byte {
	return []byte(strings.Join(lines, "\n"))
}

// dropStagedSection removes a whole `CAT`..`END CAT` section.
func dropStagedSection(lines []string, cat string) []string {
	out := lines[:0]
	i := 0
	for i < len(lines) {
		if lines[i] == cat {
			for i < len(lines) && lines[i] != "END "+cat {
				i++
			}
			i++ // skip the END line itself (or run off the end)
			continue
		}
		out = append(out, lines[i])
		i++
	}
	return out
}

// dropCopyOnlyStagedSection removes a section only when its body holds a
// `copy` directive (real extension sections skip silently in the compiler
// and are kept byte-identical).
func dropCopyOnlyStagedSection(lines []string, cat string) []string {
	si := -1
	for i, l := range lines {
		if l == cat {
			si = i
			break
		}
	}
	if si < 0 {
		return lines
	}
	j := si + 1
	for j < len(lines) && lines[j] != "END "+cat {
		j++
	}
	for _, l := range lines[si+1 : min(j, len(lines))] {
		if strings.HasPrefix(l, "copy ") {
			end := min(j+1, len(lines))
			return append(append([]string{}, lines[:si]...), lines[end:]...)
		}
	}
	return lines
}

// dropStagedBlock removes a line starting with prefix plus its `/`-continued
// physical lines (glibc block entries: map, class, kana tables).
func dropStagedBlock(lines []string, prefix string) []string {
	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if strings.HasPrefix(lines[i], prefix) {
			i++
			for i < len(lines) && endsStagedContinuation(outLastDropped(lines, i-1)) {
				i++
			}
			continue
		}
		out = append(out, lines[i])
		i++
	}
	return out
}

// outLastDropped recovers the last dropped line for the continuation test.
// The block head is at lines[i-1] on entry; as i advances, lines[i-1] is
// the previously dropped line.
func outLastDropped(lines []string, idx int) string {
	if idx < 0 || idx >= len(lines) {
		return ""
	}
	return lines[idx]
}

// endsStagedContinuation reports a glibc `/` line continuation (trailing
// blanks tolerated, as written by locale authors).
func endsStagedContinuation(line string) bool {
	return strings.HasSuffix(strings.TrimRight(line, " \t"), "/")
}

var stagedURange = regexp.MustCompile(`<U([0-9A-Fa-f]{4,8})>\.\.<U([0-9A-Fa-f]{4,8})>`)

// expandStagedURanges rewrites `<UHHHH>..<UHHHH>` in LC_CTYPE classes to
// exact `;`-separated enumerations, preserving digit width and order.
func expandStagedURanges(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, stagedURange.ReplaceAllStringFunc(l, func(m string) string {
			sub := stagedURange.FindStringSubmatch(m)
			lo := mustParseStagedHex(sub[1])
			hi := mustParseStagedHex(sub[2])
			if hi < lo {
				return m
			}
			width := max(len(sub[1]), len(sub[2]))
			members := make([]string, 0, hi-lo+1)
			for cp := lo; cp <= hi; cp++ {
				members = append(members, fmt.Sprintf("<U%0*X>", width, cp))
			}
			return strings.Join(members, ";")
		}))
	}
	return out
}

func mustParseStagedHex(s string) uint64 {
	var v uint64
	for _, c := range s {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= uint64(c - '0')
		case c >= 'a' && c <= 'f':
			v |= uint64(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= uint64(c-'A') + 10
		}
	}
	return v
}

// joinStagedCommentContinuations pre-joins a `/`-continuation whose next
// physical line starts with the comment character — replicating glibc's
// join-before-comment-stripping for strftime lines like zh_TW's d_t_fmt.
func joinStagedCommentContinuations(lines []string) []string {
	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if endsStagedContinuation(lines[i]) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "%") &&
			i+1 < len(lines) && strings.HasPrefix(lines[i+1], "%") {
			out = append(out, lines[i][:len(lines[i])-1]+lines[i+1])
			i += 2
			continue
		}
		out = append(out, lines[i])
		i++
	}
	return out
}

var stagedCharmapRange = regexp.MustCompile(`^[ \t]*<U([0-9A-Fa-f]+)>\.\.<U([0-9A-Fa-f]+)>[ \t]+((?:/x[0-9A-Fa-f]{2})+)(.*)$`)

// expandStagedCharmapRanges expands the UTF-8 charmap's range lines into one
// line per code point carrying that code point's TRUE UTF-8 encoding.
// glibc's generator emits ranges crossing continuation-byte boundaries,
// which a byte-carry expansion corrupts; re-encoding from the code point is
// what each range member denotes. A range that fails to parse (or names an
// invalid rune) is kept verbatim rather than expanded wrongly.
func expandStagedCharmapRanges(data []byte) []byte {
	lines := splitStagedLines(data)
	out := make([]string, 0, len(lines)+65536)
	for _, l := range lines {
		m := stagedCharmapRange.FindStringSubmatch(l)
		if m == nil {
			out = append(out, l)
			continue
		}
		lo := mustParseStagedHex(m[1])
		hi := mustParseStagedHex(m[2])
		if hi < lo || hi-lo > 1<<20 {
			out = append(out, l)
			continue
		}
		width := max(len(m[1]), len(m[2]))
		comment := m[4]
		ok := true
		for cp := lo; cp <= hi; cp++ {
			if !utf8.ValidRune(rune(cp)) {
				ok = false
				break
			}
		}
		if !ok {
			out = append(out, l)
			continue
		}
		for cp := lo; cp <= hi; cp++ {
			enc := []byte(string(rune(cp)))
			var esc strings.Builder
			for _, b := range enc {
				fmt.Fprintf(&esc, "/x%02x", b)
			}
			out = append(out, fmt.Sprintf("<U%0*X> %s%s", width, cp, esc.String(), comment))
		}
	}
	return joinStagedLines(out)
}

// min/max are the language builtins (go 1.21+).
