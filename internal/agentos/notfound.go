// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Command-not-found is the single largest class of agent shell failures
// (Terminal-Bench 2.0 taxonomy, 24.1%). The runtime already prints Bash's bare
// `command not found` with exit 127; an agent reading that has no idea whether
// the name is a typo, a tool that lives behind a door it can open, or genuinely
// absent — so it retries blindly.
//
// This reactive ExecHandler middleware adds ONE structured hint on exactly that
// path: when a bare command name exits 127 AND is provably unresolved (not an
// in-process coreutil, not on PATH), it emits a single bashy-hint-v1 JSON line
// on stderr carrying the resolution tier, the nearest provided name
// (did-you-mean), and the install door if one exists (a GNU coreutils name is
// reachable through the managed container). It never alters the exit status or
// the Bash error text, is inert unless BASHY_AGENTIC is set, is silenced by
// BASHY_HINTS=off, and is rate-limited to once per name per session — exactly
// like the proactive nudger and the reactive advisor it sits beside.
//
// Resolution is verified by lookup, never by the exit code alone: a real
// command that genuinely exits 127 is still on PATH or served in-process, so it
// is never misclassified as not-found.
package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/tool"
)

// notFoundHintsEnabled reports whether the command-not-found hint should fire.
// The gate is deliberately BASHY_AGENTIC: the hint is a machine-readable line
// for an agent, so it stays inert for interactive humans and without the env.
// BASHY_HINTS=off is the shared silencer for every hint surface.
func notFoundHintsEnabled() bool {
	switch strings.ToLower(os.Getenv("BASHY_HINTS")) {
	case "0", "false", "off", "no":
		return false
	}
	switch strings.ToLower(os.Getenv("BASHY_AGENTIC")) {
	case "", "0", "false", "off", "no":
		return false
	}
	return true
}

// notFoundHint is the classification of an unresolved command name, mirroring
// `bashy check`'s resolution tiers (BASHY0701 = not_found, BASHY0302 = the GNU
// coreutils container door).
type notFoundHint struct {
	tier    string // resolution tier reached: always "not_found" at runtime
	nearest string // nearest provided name (did-you-mean), "" if none close
	install string // install-door token, "" if there is no door
}

// notFoundHinter emits command-not-found hints, rate-limited once per name for
// the life of the session (shell process), like the other hint surfaces.
type notFoundHinter struct {
	provided []string        // builtin ∪ coreutil ∪ verb names (did-you-mean set)
	gnuCore  map[string]bool // names reachable via the managed GNU coreutils container
	w        io.Writer       // default sink; the handler prefers the command's own stderr
	mu       sync.Mutex
	seen     map[string]bool
}

// newNotFoundHinter builds the hinter from bashy's own command catalog so the
// did-you-mean set is exactly what this binary provides.
func newNotFoundHinter() *notFoundHinter {
	builtins, core, verbs := commandsCatalog()
	provided := make([]string, 0, len(builtins)+len(core)+len(verbs))
	provided = append(provided, builtins...)
	provided = append(provided, core...)
	provided = append(provided, verbs...)
	gnu := make(map[string]bool, len(gnuCoreutilsCommands))
	for _, n := range gnuCoreutilsCommands {
		gnu[n] = true
	}
	return &notFoundHinter{provided: provided, gnuCore: gnu, w: os.Stderr, seen: map[string]bool{}}
}

// isNotFound reports whether name is provably unresolved: not served in-process
// and not on PATH. This is the guard that keeps a real command which happens to
// exit 127 from being reported as command-not-found.
func (h *notFoundHinter) isNotFound(name string) bool {
	if name == "" {
		return false
	}
	if tool.Lookup(name) != nil {
		return false // pure-Go in-process coreutil applet
	}
	if _, err := exec.LookPath(name); err == nil {
		return false // resolves on the host PATH
	}
	return true
}

// classify produces the hint payload for an unresolved command name.
func (h *notFoundHinter) classify(name string) notFoundHint {
	base := baseName(name)
	nf := notFoundHint{tier: "not_found", nearest: nearestProvided(base, h.provided)}
	if h.gnuCore[base] {
		nf.install = "gnu-coreutils-container"
	}
	return nf
}

// notFoundLine is the bashy-hint-v1 JSON shape for a command-not-found hint. It
// reuses the established hint schema version; the kind distinguishes it from the
// proactive tool-routing hint.
type notFoundLine struct {
	Schema  string `json:"schema_version"`
	Kind    string `json:"kind"` // "command-not-found"
	Tool    string `json:"tool"`
	Tier    string `json:"tier"`
	Nearest string `json:"nearest,omitempty"`
	Install string `json:"install,omitempty"`
	Suggest string `json:"suggest"`
	Off     string `json:"off"`
}

// notFoundSuggest renders the human-readable half carried inside the JSON line.
func notFoundSuggest(name string, nf notFoundHint) string {
	var b strings.Builder
	fmt.Fprintf(&b, "`%s` is not provided by bashy (no builtin, in-process coreutil, front-door verb, or PATH match).", name)
	if nf.nearest != "" {
		fmt.Fprintf(&b, " Did you mean `%s`?", nf.nearest)
	}
	if nf.install == "gnu-coreutils-container" {
		b.WriteString(" It is available through the managed GNU coreutils container (`bashy check --allow-container` treats it as reachable).")
	}
	return b.String()
}

// emit writes at most one hint per name per session to w (the command's own
// stderr, falling back to the hinter default). Returns whether it emitted.
func (h *notFoundHinter) emit(w io.Writer, name string) bool {
	if w == nil {
		w = h.w
	}
	if w == nil {
		return false
	}
	h.mu.Lock()
	if h.seen[name] {
		h.mu.Unlock()
		return false
	}
	h.seen[name] = true
	h.mu.Unlock()

	nf := h.classify(name)
	b, _ := json.Marshal(notFoundLine{
		Schema:  nudgeSchemaVersion,
		Kind:    "command-not-found",
		Tool:    name,
		Tier:    nf.tier,
		Nearest: nf.nearest,
		Install: nf.install,
		Suggest: notFoundSuggest(name, nf),
		Off:     "BASHY_HINTS=off",
	})
	fmt.Fprintf(w, "%s\n", b)
	return true
}

// notFoundHintHandler is the post-exec ExecHandler middleware. It runs the rest
// of the chain, and when a bare command name exits 127 having provably failed to
// resolve, it appends one hint AFTER Bash's own error text. The exit status and
// that error text are returned untouched.
func notFoundHintHandler(h *notFoundHinter) func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			err := next(ctx, args)
			if len(args) == 0 {
				return err
			}
			status, ok := exitStatusOf(err)
			if !ok || status != 127 {
				return err // not a command-not-found status; say nothing
			}
			name := args[0]
			// Only bare names: a path-ish operand's 127 is a different errno
			// path (No such file / Is a directory), not a PATH miss, and Bash
			// already said so precisely.
			if strings.ContainsAny(name, `/\`) {
				return err
			}
			if !h.isNotFound(name) {
				return err
			}
			h.emit(handlerStderr(ctx), name)
			return err
		}
	}
}

// nearestProvided returns the closest provided name to want within a small edit
// distance, or "" when nothing is close enough to be a useful did-you-mean. Ties
// resolve to the first candidate in the (sorted) provided set, so the result is
// deterministic.
func nearestProvided(want string, provided []string) string {
	if want == "" {
		return ""
	}
	// Scale the tolerance with the name length so short names are not matched to
	// unrelated short names, while keeping a hard ceiling of 2 edits.
	max := 2
	if len(want) <= 3 {
		max = 1
	}
	best := ""
	bestDist := max + 1
	for _, cand := range provided {
		if cand == want {
			continue // an exact match is not a "did you mean"
		}
		// A cheap length prefilter keeps the full DP off obviously-far names.
		if abs(len(cand)-len(want)) > max {
			continue
		}
		d := levenshtein(want, cand)
		if d < bestDist {
			bestDist, best = d, cand
		}
	}
	if bestDist <= max {
		return best
	}
	return ""
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// levenshtein is the classic edit distance (insert/delete/substitute, cost 1),
// computed with a single rolling row.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
