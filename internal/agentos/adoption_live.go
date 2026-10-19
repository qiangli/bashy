// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Live adoption counters (Sprint 446 S10): which triggers agents actually use,
// which hints they were shown, and which hints they followed.
//
// No new store and no new verb. The counters ride the existing execution
// record (pkg/execlog, written by exechist.go) as `adopt:` atoms in the
// record's effects list, so `graph history --json` already exposes them:
//
//	adopt:agent=<tool>         the agent tool the record is attributed to
//	adopt:option=<--opt>       a common-options vocabulary flag was used
//	adopt:lens / adopt:adapt   the representation filter / adapter front door
//	adopt:hint-shown=<cmd>     a tool hint for <cmd> was emitted on this command
//	adopt:hint-followed=<cmd>  this command used the suggestion of that hint
//
// "Followed" is deliberately narrow: the SAME agent tool's NEXT call uses the
// suggestion. An agent sees a hint only after its call returns, so commands in
// the call that showed it can never follow it; and only the next call counts,
// so a suggestion picked up much later is not credited to the hint. One agent
// call is one bashy process, so a shown hint is handed to the next process of
// the same episode through a small pending file next to the other hint state
// (see adoptionPendingDir). The file is correlation state, never a counter:
// counts are always derived from the records.
//
// Everything here is an observer. It never changes a command, its output or
// its exit; hints keep their BASHY_HINTS gate and their one-per-tool-per-
// session budget (the nudger decides; this file only notes what it decided).
package agentos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/policy/audit"
)

const (
	adoptPrefix        = "adopt:"
	adoptPendingSchema = "bashy-hint-pending-v1"
	maxAdoptQueue      = 64 // bounds annotations for commands that never reach the recorder
)

// adoptionUses returns the argv-derived adoption atoms for one command:
// common-option flags, and lens/adapt. Pure, so the front door and the shell
// record the same atoms for the same argv.
func adoptionUses(argv []string) []string {
	v := verbArgv(argv)
	if len(v) == 0 {
		return nil
	}
	var atoms []string
	switch v[0] {
	case "lens", "adapt":
		atoms = append(atoms, adoptPrefix+v[0])
	}
	for _, o := range CommonOptionsVocabulary {
		if hasLongFlag(v[1:], strings.TrimPrefix(o.Option, "--")) {
			atoms = append(atoms, adoptPrefix+"option="+o.Option)
		}
	}
	return atoms
}

// verbArgv strips a leading `bashy` (bare or by path) so `bashy lens …` and a
// front-door `lens …` read the same, and reduces argv[0] to its base name.
func verbArgv(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	if baseName(argv[0]) == "bashy" || argv[0] == bashySelfPath() {
		if len(argv) < 2 || strings.HasPrefix(argv[1], "-") {
			return nil // bashy operating on itself, not a verb
		}
		argv = argv[1:]
	}
	out := slices.Clone(argv)
	out[0] = baseName(out[0])
	return out
}

// suggestionForm is one executable shape a hint suggests: a command word path
// and the long flags it names. A flag-only suggestion (`--agentic`) applies to
// the hinted command itself.
type suggestionForm struct {
	verb  []string
	flags []string
}

var (
	backtickSpan = regexp.MustCompile("`([^`]+)`")
	verbWord     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// suggestionForms extracts the backticked command shapes from a hint. Words
// are taken up to the first flag or placeholder (DIR, CMD, <symbol>, FILE);
// long flags are taken up to a `--` separator, which starts the wrapped
// command's own argv.
func suggestionForms(tool, suggest string) []suggestionForm {
	var forms []suggestionForm
	for _, m := range backtickSpan.FindAllStringSubmatch(suggest, -1) {
		toks := strings.Fields(m[1])
		if len(toks) > 0 && toks[0] == "bashy" {
			toks = toks[1:]
		}
		var f suggestionForm
		inVerb := true
		for _, t := range toks {
			if t == "--" {
				break
			}
			if inVerb && verbWord.MatchString(t) {
				f.verb = append(f.verb, t)
				continue
			}
			inVerb = false
			if strings.HasPrefix(t, "--") && len(t) > 2 {
				name, _, _ := strings.Cut(t, "=")
				f.flags = append(f.flags, name)
			}
		}
		if len(f.verb) == 0 {
			if len(f.flags) == 0 {
				continue
			}
			f.verb = []string{tool}
		}
		forms = append(forms, f)
	}
	return forms
}

// usesSuggestion reports whether argv runs one of the suggested shapes.
func usesSuggestion(argv []string, forms []suggestionForm) bool {
	v := verbArgv(argv)
	for _, f := range forms {
		if len(v) < len(f.verb) || !slices.Equal(v[:len(f.verb)], f.verb) {
			continue
		}
		ok := true
		for _, fl := range f.flags {
			if !hasLongFlag(v[len(f.verb):], strings.TrimPrefix(fl, "--")) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// pendingHint is a hint shown in one call, awaiting the next call.
type pendingHint struct {
	Agent   string `json:"agent"`
	Tool    string `json:"tool"`
	Suggest string `json:"suggest"`
	PID     int    `json:"pid"`

	followed bool
}

// adoptionLive is the per-process session tally: hints this call showed, the
// previous call's hints this call may follow, and atoms waiting for the
// recorder to write them onto the matching command's record.
type adoptionLive struct {
	mu      sync.Mutex
	agent   string
	pid     int
	ppid    int    // a nested bashy inside the call that showed a hint is not its next call
	path    string // pending file for this episode; "" keeps state in-process only
	loaded  bool
	prior   []*pendingHint      // the previous call's hints (candidates to follow)
	shown   []pendingHint       // hints shown in this call (persisted for the next)
	queued  map[string][]string // argv key -> atoms for that command's record
	carried []string            // atoms from builtins, which have no record of their own
}

func newAdoptionLive(agent, path string) *adoptionLive {
	return &adoptionLive{agent: agent, pid: os.Getpid(), ppid: os.Getppid(), path: path, queued: map[string][]string{}}
}

// processAdoption is the one tally per process, shared by the audit-callback
// hinters (which observe) and the recorder (which writes). Nil when the
// execution record is off: with nowhere to write, there is nothing to tally.
var processAdoption = sync.OnceValue(func() *adoptionLive {
	if !execHistEnabled() {
		return nil
	}
	return newAdoptionLive(onceAdoptionAgent(), adoptionPendingPath(onceEpisode()))
})

var onceAdoptionAgent = sync.OnceValue(adoptionAgent)

// adoptionAgent names the agent tool records are attributed to: the launcher's
// declared tool, else the registry's env-marker detection.
func adoptionAgent() string {
	if a := audit.ActorFromEnv().Agent; a != "" {
		return a
	}
	if t, ok := fleet.DetectTool(); ok && t != "" {
		return t
	}
	return "unknown"
}

// adoptionPendingDir sits next to the inbox hint state, under the same
// persistence switch (BASHY_ADVISOR_NOMEM) and override (BASHY_HINTS_STATE).
func adoptionPendingDir() string {
	p := inboxHintsStatePath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "hint-pending")
}

var unsafeEpisodeRune = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func adoptionPendingPath(episode string) string {
	dir := adoptionPendingDir()
	episode = strings.Trim(unsafeEpisodeRune.ReplaceAllString(episode, "_"), ".")
	if dir == "" || episode == "" {
		return ""
	}
	return filepath.Join(dir, episode+".json")
}

// adoptKey matches the audit event's argv to the recorder's, which has had a
// self-path `bashy` stripped (commandArgv); verbArgv normalizes both.
func adoptKey(argv []string) string {
	if v := verbArgv(argv); v != nil {
		argv = v
	}
	return strings.Join(argv, "\x00")
}

// observe runs for every audited command BEFORE the hinters: it credits a
// follow when this command uses a previous call's suggestion. The first
// observation consumes the pending file, so only this (the next) call can
// follow those hints.
func (a *adoptionLive) observe(argv []string, builtin bool) {
	if a == nil || len(argv) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.loadLocked()
	for _, p := range a.prior {
		if p.followed || p.Agent != a.agent || p.PID == a.pid || p.PID == a.ppid {
			continue
		}
		if usesSuggestion(argv, suggestionForms(p.Tool, p.Suggest)) {
			p.followed = true
			a.noteLocked(argv, builtin, adoptPrefix+"hint-followed="+p.Tool)
		}
	}
}

// hintShown notes a hint the nudger actually emitted for argv, and hands it to
// the next call.
func (a *adoptionLive) hintShown(argv []string, builtin bool, tool, suggest string) {
	if a == nil || len(argv) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.loadLocked()
	a.noteLocked(argv, builtin, adoptPrefix+"hint-shown="+tool)
	a.shown = append(a.shown, pendingHint{Agent: a.agent, Tool: tool, Suggest: suggest, PID: a.pid})
	a.saveLocked()
}

func (a *adoptionLive) noteLocked(argv []string, builtin bool, atom string) {
	if builtin {
		if len(a.carried) < maxAdoptQueue {
			a.carried = append(a.carried, atom)
		}
		return
	}
	k := adoptKey(argv)
	if _, ok := a.queued[k]; !ok && len(a.queued) >= maxAdoptQueue {
		return
	}
	a.queued[k] = append(a.queued[k], atom)
}

// take returns the atoms owed to this command's record: those noted for its
// argv, plus any carried from builtins since the last record.
func (a *adoptionLive) take(argv []string) []string {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	k := adoptKey(argv)
	atoms := append(a.carried, a.queued[k]...)
	a.carried = nil
	delete(a.queued, k)
	return atoms
}

func (a *adoptionLive) loadLocked() {
	if a.loaded {
		return
	}
	a.loaded = true
	if a.path == "" {
		return
	}
	b, err := os.ReadFile(a.path)
	if err != nil {
		return
	}
	_ = os.Remove(a.path) // consumed: the call after this one may not follow these
	var f struct {
		Schema string        `json:"schema_version"`
		Hints  []pendingHint `json:"hints"`
	}
	if json.Unmarshal(b, &f) != nil || f.Schema != adoptPendingSchema {
		return
	}
	for i := range f.Hints {
		a.prior = append(a.prior, &f.Hints[i])
	}
}

// saveLocked persists this call's hints for the next call (temp + rename),
// best-effort like every other hint state.
func (a *adoptionLive) saveLocked() {
	if a.path == "" {
		return
	}
	b, err := json.Marshal(struct {
		Schema string        `json:"schema_version"`
		Hints  []pendingHint `json:"hints"`
	}{adoptPendingSchema, a.shown})
	if err != nil {
		return
	}
	dir := filepath.Dir(a.path)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".pending-*.json")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	if cerr := tmp.Close(); werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return
	}
	_ = os.Rename(tmp.Name(), a.path)
}

// adoptionAtoms is everything the recorder appends to one record's effects:
// the argv-derived uses, the hint atoms owed to it, and the agent attribution
// whenever there is anything to attribute.
func adoptionAtoms(a *adoptionLive, argv []string) []string {
	atoms := append(adoptionUses(argv), a.take(argv)...)
	if len(atoms) == 0 {
		return nil
	}
	agent := onceAdoptionAgent()
	if a != nil {
		agent = a.agent
	}
	return append([]string{adoptPrefix + "agent=" + agent}, atoms...)
}
