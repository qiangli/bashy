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
// in-process coreutil, not a registered command, not on PATH), it emits a single
// bashy-hint-v1 JSON line on stderr carrying the resolution tier, the nearest
// provided name (did-you-mean), and the install door if one exists (a GNU
// coreutils name is reachable through the managed container). It never alters
// the exit status or the Bash error text, is inert unless BASHY_AGENTIC is set,
// is silenced by BASHY_HINTS=off, and is rate-limited to once per name per
// session — exactly like the proactive nudger and the reactive advisor it sits
// beside.
//
// Resolution is verified by lookup, never by the exit code alone — and in the
// SAME context the shell itself resolves: the default exec handler looks a bare
// name up with interp.LookPathDir(hc.Dir, hc.Env, name), so a PATH assigned
// inside the script or a relative PATH entry read after a `cd` resolves exactly
// as the command does. os/exec.LookPath against the process environment cannot
// see either, and would both (a) report a REAL command as not-found when it
// lives only on the shell-local PATH and (b) suppress the hint for a genuine
// shell-local miss that the host PATH happens to satisfy. The lookup is captured
// BEFORE the command runs, so a command that exists now and then removes itself
// (or rewrites PATH) and exits 127 keeps its genuine 127 rather than being
// reported as not-found.
package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/tool"
	"github.com/qiangli/yoke/pkg/recommend"
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
	tier    string // resolution tier reached: "not_found", or "container" when a door exists
	nearest string // nearest provided name (did-you-mean), "" if none close
	install string // install-door token, "" if there is no door
}

// notFoundHinter emits command-not-found hints, rate-limited once per name for
// the life of the session (shell process), like the other hint surfaces.
type notFoundHinter struct {
	provided []string        // builtin ∪ coreutil ∪ verb names (did-you-mean set)
	handled  map[string]bool // front-door verb / self-shim names bashy dispatches off PATH
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
	// Front-door verbs (and the `sh` self-shim) are dispatched by bashy itself,
	// not resolved on PATH — the same rung `bashy check` reports as BASHY0202
	// ("verb"). They must count as resolved, or a real verb that reaches the
	// exec handler and exits 127 would be mislabelled "not provided by bashy".
	// This mirrors the analyzer's verb set (check.go: verbs ∪ {"sh"}; "docker"
	// is already in verbs).
	handled := make(map[string]bool, len(verbs)+1)
	for _, n := range verbs {
		handled[n] = true
	}
	handled["sh"] = true
	return &notFoundHinter{provided: provided, handled: handled, gnuCore: gnu, w: os.Stderr, seen: map[string]bool{}}
}

// resolves reports whether name resolves to something runnable in the SAME
// context the shell's own exec chain uses: an in-process coreutil applet, a
// registered command, or a PATH hit under the interpreter's current environment
// and directory (interp.LookPathDir(hc.Dir, hc.Env, name) — the exact call the
// default exec handler makes). Using the handler context rather than the process
// environment is the whole correction: a PATH set inside the script and a
// relative PATH entry after a `cd` resolve as the command does, and a genuine
// shell-local miss is not masked by whatever the host PATH happens to carry.
func (h *notFoundHinter) resolves(ctx context.Context, name string) bool {
	if name == "" {
		return false
	}
	if h.handled[name] {
		return true // a bashy front-door verb / self-shim: dispatched, never PATH-resolved
	}
	if tool.Lookup(name) != nil {
		return true // pure-Go in-process coreutil applet (coreutilsshell.Handler)
	}
	if _, ok := registeredLookup(name); ok {
		return true // a `bashy commands add` registered command (registeredHandler)
	}
	hc := interp.HandlerCtx(ctx)
	if _, err := interp.LookPathDir(hc.Dir, hc.Env, name); err == nil {
		return true // resolves on the shell's own PATH, from its own cwd
	}
	return false
}

// classify produces the hint payload for an unresolved command name.
func (h *notFoundHinter) classify(name string) notFoundHint {
	base := baseName(name)
	nf := notFoundHint{tier: "not_found", nearest: nearestProvided(base, h.provided)}
	if h.gnuCore[base] {
		// Reachable through the managed GNU coreutils container: the same
		// BASHY0302 door `bashy check` reports as Kind "container". The tier
		// reflects that door rather than a bare not_found.
		nf.tier = "container"
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
			// Only bare names are candidates: a path-ish operand's 127 is a
			// different errno path (No such file / Is a directory), not a PATH
			// miss, and Bash already said so precisely.
			var name string
			bare := false
			if len(args) > 0 {
				name = args[0]
				// An empty operand is never a nameable command-not-found: skip
				// it so the hint never fires with a blank tool name.
				bare = name != "" && !strings.ContainsAny(name, `/\`)
			}
			// Resolve BEFORE running, in the shell's own context: a command
			// that resolves now but removes itself (or rewrites PATH) and then
			// exits 127 must keep its genuine 127, not be reported as not-found.
			resolvedBefore := bare && h.resolves(ctx, name)

			err := next(ctx, args)
			if !bare {
				return err
			}
			status, ok := exitStatusOf(err)
			if !ok || status != 127 {
				return err // not a command-not-found status; say nothing
			}
			if resolvedBefore {
				return err // a real command that resolved; its 127 is its own
			}
			h.emit(handlerStderr(ctx), name)
			return err
		}
	}
}

// nearestProvided returns the closest provided name to want, or "" when nothing
// is close enough to be a useful did-you-mean. It defers to the shared nudge
// recommender (pkg/recommend) — the same lexical-similarity ranking the
// not-found-target advisor uses — rather than carrying a second edit-distance
// implementation.
func nearestProvided(want string, provided []string) string {
	recs := recommend.Recommend(want, provided, 1)
	if len(recs) == 0 {
		return ""
	}
	return recs[0].Name
}
