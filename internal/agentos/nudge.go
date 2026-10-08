// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// The proactive half of the nudge subsystem (the advisor is the reactive half).
// When an agent uses a legacy tool that has a better agentic counterpart, bashy
// emits ONE rate-limited hint pointing at it — never changing the tool's
// behavior. First nudge: cd/pushd/popd → suggest `awd` (run one command
// elsewhere without leaking the shell's cwd).
//
// Prime invariant: help, don't obstruct. Nudges are stderr-only (stdout stays
// pure data), rate-limited to once per (tool, session) so they teach without
// becoming noise, and fully silenceable. They are observers — they never block
// or alter the command.
package agentos

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/nudge"
	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/yoke/pkg/fleet"
)

const nudgeSchemaVersion = nudge.SchemaVersion

// nudger emits proactive tool-hints, rate-limited via the shared session memory.
type nudger struct {
	agent bool
	mem   *memory
	w     io.Writer
}

// newNudger shares the advisor's memory so hint rate-limiting is per session.
func newNudger(mem *memory) *nudger {
	return &nudger{agent: weavecli.IsAgentDriven(), mem: mem, w: os.Stderr}
}

// onAudit is the [interp.WithAuditHandler] callback. It fires once per simple
// command (post-expansion); we act only on watched tools, once per session.
// Builtins (cd/pushd/popd) get the awd nudge; external search tools (grep/find)
// get an argv-conditioned routing hint toward --agentic / the code-intel verbs.
func (n *nudger) onAudit(ev interp.AuditEvent) {
	if len(ev.Args) == 0 {
		return
	}
	name := ev.Args[0]
	// Rules live in coreutils/pkg/nudge — the single source of truth shared with
	// ycode and any other consumer of the in-process userland. bashy keeps its
	// own session-memory rate-limiting + emit below.
	suggest := nudge.Suggest(ev.Args, ev.IsBuiltin)
	if suggest == "" {
		return
	}
	if n.mem != nil && !n.mem.firstHint("nudge:"+name) {
		return // already nudged for this tool this session
	}
	n.emit(name, suggest)
}

// nudgeLine is the agent-mode JSON shape (one line on stderr).
type nudgeLine struct {
	Schema  string `json:"schema_version"`
	Kind    string `json:"kind"` // "hint"
	Tool    string `json:"tool"`
	Suggest string `json:"suggest"`
	Off     string `json:"off"`
}

func (n *nudger) emit(tool, suggest string) {
	if n.w == nil {
		return
	}
	if n.agent {
		b, _ := json.Marshal(nudgeLine{
			Schema:  nudgeSchemaVersion,
			Kind:    "hint",
			Tool:    tool,
			Suggest: suggest,
			Off:     "BASHY_HINTS=off",
		})
		fmt.Fprintf(n.w, "%s\n", b)
		return
	}
	fmt.Fprintf(n.w, "─── bashy hint ─── %s (silence: BASHY_HINTS=off)\n", suggest)
}

// agenticDisabled reports the master off-switch: BASHY_AGENTIC set to an off-ish
// value turns off agentic defaults AND all nudges/advice.
func agenticDisabled() bool {
	switch strings.ToLower(os.Getenv("BASHY_AGENTIC")) {
	case "0", "false", "off", "no":
		return true
	}
	return false
}

// hintsEnabled reports whether proactive tool-hints should fire. BASHY_AGENTIC
// off is the master kill; otherwise BASHY_HINTS is the explicit control
// (off-ish silences, on-ish forces on); unset defaults to agent mode only.
func hintsEnabled() bool {
	if agenticDisabled() {
		return false
	}
	switch strings.ToLower(os.Getenv("BASHY_HINTS")) {
	case "0", "false", "off", "no":
		return false
	case "1", "true", "on", "yes":
		return true
	}
	return weavecli.IsAgentDriven()
}

// unread mail hints ("you have mail") live here, on the existing hint engine:
// the same stderr-only channel as tool nudges (agent mode: one JSON hint
// line; human mode: one prose line), so structured stdout/NDJSON stays clean.
// The unread view is the managed-turn-delivery snapshot (snapshotUnifiedInbox)
// read strictly read-only — its ack closures are never invoked — so a hint
// never consumes mail. Arrival scoping comes from the persisted fingerprint
// state (inboxHintArrivals): a hint fires once per store fingerprint, which
// suppresses duplicates within a turn and re-hints when new mail arrives.

// unreadHintSkipVerbs are front-door verbs that never trigger the ambient
// unread probe. Mail readers (inbox/mb/notify) are about to see the mail
// itself; serve/mcp own long-lived or protocol stdio where a hint is noise;
// inbox-hook is the hook entrypoint below — probing there would announce the
// arrival on stderr AND mark it hinted, silencing the hook's own stdout
// envelope for the very turn it was installed to serve.
var unreadHintSkipVerbs = map[string]bool{
	"inbox": true, "mb": true, "messages": true, "notify": true,
	"serve": true, "mcp": true, "inbox-hook": true,
}

// unreadSummary is the attributable shape behind one hint: how many, from
// which sources, from whom — never bodies.
type unreadSummary struct {
	count    int
	bySource []string // "mb:2", "meet/room:1", ... in first-seen order
	senders  []string // distinct non-empty From, capped, in first-seen order
}

// summarizeUnreadHint reduces a read-only snapshot to its attributable shape.
// Empty batches (no unread) report ok=false: silence, not "no mail".
func summarizeUnreadHint(batch inboxBatch) (unreadSummary, bool) {
	var s unreadSummary
	counts := map[string]int{}
	var order []string
	seenSender := map[string]bool{}
	for _, e := range batch.events {
		s.count++
		src := e.Source
		if e.Room != "" {
			src += "/" + e.Room
		}
		if counts[src] == 0 {
			order = append(order, src)
		}
		counts[src]++
		from := strings.TrimSpace(e.From)
		if from == "" || strings.EqualFold(from, "unknown") {
			continue
		}
		if !seenSender[from] {
			seenSender[from] = true
			if len(s.senders) < 3 {
				s.senders = append(s.senders, from)
			}
		}
	}
	if s.count == 0 {
		return unreadSummary{}, false
	}
	for _, src := range order {
		s.bySource = append(s.bySource, fmt.Sprintf("%s:%d", src, counts[src]))
	}
	return s, true
}

// formatUnreadHint renders the concise model-visible line: unread count,
// source attribution, sender attribution, and the one reader to run.
func formatUnreadHint(s unreadSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "you have %d unread (%s)", s.count, strings.Join(s.bySource, ", "))
	if len(s.senders) > 0 {
		fmt.Fprintf(&b, " from %s", strings.Join(s.senders, ", "))
	}
	b.WriteString(" — run `bashy inbox`")
	return b.String()
}

// liveUnreadSnapshot is the read-only unread view, shared with managed turn
// delivery. includeBus=false matches the turn preamble: bus pending is drained
// by bus.TurnPreamble itself and must not be double-reported here.
func liveUnreadSnapshot(reader string) (inboxBatch, error) {
	return snapshotUnifiedInbox(reader, 0, false)
}

// checkUnreadArrivals is the arrival-scoped gate. fp is the store fingerprint
// for reader (fpOK=false when it could not be sampled); snapshot reads unread
// mail without consuming it; refingerprint re-samples the stores after a read
// (nil skips this). It returns the hint text ("" when silent) and the next
// persisted state, plus whether a snapshot ran (the caller saves state only
// then, so a failed snapshot retries next turn instead of suppressing).
//
// The re-sample matters because the first read lazily materializes cursor
// files, which moves the fingerprint without any mail arriving: recording the
// pre-read generation would re-hint on the very next turn. The recorded
// generation is therefore the post-read one, re-reading (bounded) while the
// store keeps moving under us so a racing arrival is included, not missed.
func checkUnreadArrivals(reader string, fp uint64, fpOK bool, st inboxHintArrivals, snapshot func(string) (inboxBatch, error), refingerprint func(string) (uint64, bool)) (string, inboxHintArrivals, bool) {
	if fpOK && fp != 0 {
		if st.Hinted[reader] == fp || st.Probed[reader] == fp {
			return "", st, false // announced, or probed quiet since the last change
		}
	}
	pre, preOK := fp, fpOK
	var batch inboxBatch
	var err error
	for tries := 0; tries < 3; tries++ {
		if batch, err = snapshot(reader); err != nil {
			return "", st, false
		}
		if refingerprint == nil {
			break
		}
		post, postOK := refingerprint(reader)
		if !postOK || post == 0 || (preOK && post == pre) {
			break
		}
		pre, preOK = post, postOK
	}
	next := inboxHintArrivals{
		Probed: maps.Clone(st.Probed),
		Hinted: maps.Clone(st.Hinted),
	}
	if next.Probed == nil {
		next.Probed = map[string]uint64{}
	}
	if next.Hinted == nil {
		next.Hinted = map[string]uint64{}
	}
	if preOK && pre != 0 {
		next.Probed[reader] = pre
	}
	sum, ok := summarizeUnreadHint(batch)
	if !ok {
		return "", next, true
	}
	if preOK && pre != 0 {
		next.Hinted[reader] = pre
	}
	return formatUnreadHint(sum), next, true
}

// resolveHintReader scopes a hint exactly like `bashy inbox` would: the same
// reader resolution, hence the same instance cursors and the same authorized
// role mail — never another instance's. retired reports a principal that names
// an agent with no live fleet definition: retired mail is archived and never
// inherited, so a stale session stays silent. The login-user legacy fallback
// (no agent principal) is unaffected: it reads its own mail as before.
func resolveHintReader(as string) (reader string, retired bool, err error) {
	reader, err = resolveInboxReader(as)
	if err != nil {
		return "", false, err
	}
	principal := strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
	if !strings.Contains(principal, "agent/") {
		return reader, false, nil
	}
	if _, ok := fleet.New().Agent(reader); !ok {
		return "", true, nil
	}
	return reader, false, nil
}

// maybeHintUnreadMail is the narrow dispatch callback: every ordinary `bashy`
// command surfaces the same unread hint on stderr (stdout stays pure data).
// It runs before the command so one choke point covers all verbs; mail
// readers and protocol verbs are skipped above.
func maybeHintUnreadMail(argv []string) {
	if !hintsEnabled() {
		return
	}
	emitUnreadHint(argv, os.Stderr, weavecli.IsAgentDriven())
}

// emitUnreadHint is the testable body behind maybeHintUnreadMail: same gate,
// caller-chosen writer and mode.
func emitUnreadHint(argv []string, w io.Writer, agent bool) {
	if !hintsEnabled() {
		return
	}
	if len(argv) == 0 {
		return
	}
	verb := argv[0]
	if verb == "" || verb[0] == '-' || verb[0] == '+' || strings.ContainsRune(verb, '/') {
		return
	}
	if unreadHintSkipVerbs[verb] {
		return
	}
	reader, retired, err := resolveHintReader("")
	if err != nil || retired || reader == "" {
		return
	}
	fp, fpOK := inboxSourcesFingerprint(reader)
	st := loadInboxHintArrivals()
	hint, next, ran := checkUnreadArrivals(reader, fp, fpOK, st, liveUnreadSnapshot, inboxSourcesFingerprint)
	if ran {
		next.save()
	}
	if hint == "" {
		return
	}
	(&nudger{agent: agent, w: w}).emit("inbox", hint)
}

// hintsForceOff reports the explicit kill switches (BASHY_AGENTIC master off,
// or BASHY_HINTS off-ish). Unlike hintsEnabled it never defaults on: the
// hook entrypoint honors an operator's explicit off without requiring an
// agent-driven session to speak at all.
func hintsForceOff() bool {
	if agenticDisabled() {
		return true
	}
	switch strings.ToLower(os.Getenv("BASHY_HINTS")) {
	case "0", "false", "off", "no":
		return true
	}
	return false
}

// sniffHookEvent names the turn-boundary event a hook fired for, accepting
// both harnesses' stdin field casings (Claude Code and Codex both send the
// event name in the hook JSON object). "" means the input carried no name.
func sniffHookEvent(data []byte) string {
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	for _, k := range []string{"hook_event_name", "hookEventName", "hook_event", "hookEvent"} {
		if v, _ := m[k].(string); strings.TrimSpace(v) != "" {
			name := strings.TrimSpace(v)
			switch strings.ToLower(name) {
			case "sessionstart":
				return "SessionStart"
			case "userpromptsubmit":
				return "UserPromptSubmit"
			}
			return name
		}
	}
	return ""
}

// hookEnvelope renders the model-visible hook output both harnesses honor:
// hookSpecificOutput.additionalContext. Empty hint never reaches here — the
// hook stays silent instead, so an empty turn costs the model zero tokens.
func hookEnvelope(event, hint string) string {
	b, _ := json.Marshal(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     event,
			"additionalContext": hint,
		},
	})
	return string(b)
}

// dispatchInboxHook runs `bashy inbox-hook [--as NAME] [--for EVENT]`, the
// turn-boundary entrypoint installed into Claude Code (SessionStart /
// UserPromptSubmit settings hooks) and Codex (SessionStart / UserPromptSubmit
// inline hooks) by `bashy install-agent <agent> --hooks`. It prints at most
// one JSON envelope on stdout when unread mail waits, else exits 0 silent.
// Stdin carries the harness hook JSON (used only to name the event); stdout is
// the model-visible channel here, so diagnostics stay on stderr and errors
// that would pollute context exit silently.
func dispatchInboxHook(args []string) int {
	return dispatchInboxHookTo(args, os.Stdin, os.Stdout, os.Stderr)
}

func dispatchInboxHookTo(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inbox-hook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	as := fs.String("as", "", "read as this identity (same as `bashy inbox --as`)")
	forName := fs.String("for", "", "hook event name (default: sniffed from hook JSON on stdin, else SessionStart)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if hintsForceOff() {
		return 0
	}
	event := strings.TrimSpace(*forName)
	var payload []byte
	if event == "" && stdin != nil {
		payload, _ = io.ReadAll(stdin)
		event = sniffHookEvent(payload)
	}
	if event == "" {
		event = "SessionStart"
	}
	reader, retired, err := resolveHintReader(strings.TrimSpace(*as))
	if err != nil || retired || reader == "" {
		if err != nil {
			fmt.Fprintf(stderr, "inbox-hook: no attributable identity (%v)\n", err)
		} else if retired {
			fmt.Fprintf(stderr, "inbox-hook: identity is retired; staying silent\n")
		}
		return 0
	}
	fp, fpOK := inboxSourcesFingerprint(reader)
	st := loadInboxHintArrivals()
	hint, next, ran := checkUnreadArrivals(reader, fp, fpOK, st, liveUnreadSnapshot, inboxSourcesFingerprint)
	if ran {
		next.save()
	}
	if hint == "" {
		return 0
	}
	fmt.Fprintln(stdout, hookEnvelope(event, hint))
	return 0
}
