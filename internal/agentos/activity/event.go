// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package activity is the shared activity-event contract for Bashy
// subsystems: one compact envelope, one interest-routing matrix, and one
// delivery path onto the EXISTING durable notification and wake primitives.
//
// # Why a contract and not another channel
//
// Before this package each subsystem that wanted to tell somebody something
// grew its own notice type, its own dedup key and its own publish call —
// pkg/weave/weave_notice.go, pkg/kb/bus.go and pkg/chat/coach.go each
// re-derived the same five decisions independently. Three spellings of one
// idea drift, and the drift is invisible: a subsystem that forgets the
// timeline dedup check republishes on every recovery, and a subsystem that
// forgets the wake leaves a durable message nobody reads until the next turn
// that happens to look. This package makes those five decisions once.
//
// # It is NOT a mailbox
//
// The durable mailbox is pkg/bus (bashy#10): bus.Publish appends to the
// append-only room timeline, bus.EnsureSubscription gives an offline recipient
// an inbox, bus.SteerLive wakes a live one, and `bashy inbox` is the read
// side. This package adds NO second mailbox. What it keeps on disk is an
// OUTBOX — an emit-side journal whose only jobs are deduplication, per-source
// sequencing and crash recovery. It holds no per-recipient read state, because
// the moment it did there would be two answers to "have I read this" and the
// wrong one would be the newer.
//
// # The envelope cannot carry a body
//
// The requirement is that an activity event never carries full bodies,
// prompts, secrets, terminal output or diffs. That is enforced structurally
// rather than by review: Event has no free-text body field at all, every
// string is length-capped, control characters are refused, and Action and
// Status come from closed vocabularies. There is nowhere to put a diff. A
// policed rule is a rule that holds until somebody is in a hurry; an absent
// field holds forever.
package activity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SchemaVersion identifies the envelope wire format.
const SchemaVersion = "bashy-activity-v1"

// Known sources — the subsystems this contract ships with. RegisterSource
// widens the set for a subsystem added later (bashy#11 wires its own).
const (
	SourceMB     = "mb"
	SourceMeet   = "meet"
	SourceWeave  = "weave"
	SourceSprint = "sprint"
	SourceTodo   = "todo"
	SourceInbox  = "inbox"
	SourcePing   = "ping"
	SourceNotify = "notify"

	// SourceActivity is RESERVED and never routes to anybody. It is the
	// loop-prevention floor: delivering an activity event necessarily touches
	// the bus, and if bus traffic could itself be announced as activity the
	// first event would be the last thing this host ever did.
	SourceActivity = "activity"
)

// Actions. The four CRUD verbs plus the lifecycle verbs a run/story/meeting
// needs. The set is closed: an open action vocabulary is an open body field
// with extra steps.
const (
	ActionCreate = "create"
	ActionRead   = "read"
	ActionUpdate = "update"
	ActionDelete = "delete"

	ActionStart  = "start"
	ActionFinish = "finish"
	ActionFail   = "fail"
	ActionCancel = "cancel"
	ActionBlock  = "block"
)

// Status is the MINIMAL outcome vocabulary. It answers "did it work", not
// "what happened" — the object reference is how a recipient finds out what
// happened, and it costs no tokens until somebody actually wants to know.
const (
	StatusOK      = "ok"
	StatusFailed  = "failed"
	StatusBlocked = "blocked"
	StatusPending = "pending"
)

// Field limits. These are what make the envelope compact rather than
// aspirationally compact. They are refusals, never truncations: silently
// shortening an object reference produces a pointer that resolves to nothing,
// which is worse than the emit failing loudly at the call site that can fix it.
const (
	MaxFieldBytes = 96
	MaxActorBytes = 64
	MaxListLen    = 32
	MaxHop        = 3
)

// Scope is where the event happened. Every field is optional; together they
// are the membership axis of the routing matrix.
type Scope struct {
	Repo   string `json:"repo,omitempty"`
	Sprint string `json:"sprint,omitempty"`
	Topic  string `json:"topic,omitempty"`
	Room   string `json:"room,omitempty"`
}

// Empty reports a scope that names no place.
func (s Scope) Empty() bool {
	return s.Repo == "" && s.Sprint == "" && s.Topic == "" && s.Room == ""
}

// Event is one compact activity fact.
//
// The routing inputs (Mentions/Owner/Assignees/DependsOn/Members) are part of
// the envelope on purpose. Routing has to be answerable from the event alone:
// a router that had to call back into the emitting subsystem to ask "who owns
// this" would make delivery depend on that subsystem still being alive, and
// the events that matter most are exactly the ones emitted as something dies.
type Event struct {
	Schema string    `json:"schema"`
	ID     string    `json:"id"`
	Seq    int64     `json:"seq"`
	At     time.Time `json:"at"`

	Source string `json:"source"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Noun   string `json:"noun"`
	Object string `json:"object"`
	Scope  Scope  `json:"scope,omitzero"`
	Status string `json:"status,omitempty"`

	// --- routing inputs -----------------------------------------------------

	Mentions  []string `json:"mentions,omitempty"`
	Owner     string   `json:"owner,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
	Members   []string `json:"members,omitempty"`

	// --- loop prevention ----------------------------------------------------

	// Cause is the id of the event that led to this one, and Hop is how many
	// links back that chain runs. An emitter that reacts to activity by
	// emitting activity must carry both forward; Validate refuses at MaxHop.
	Cause string `json:"cause,omitempty"`
	Hop   int    `json:"hop,omitempty"`

	// Token is the caller's transaction-boundary discriminator — whatever makes
	// THIS commit of THIS object distinct from the next one (a terminal state,
	// a revision, a monotonic counter). It is folded into ID, which is what
	// makes a replayed emit idempotent.
	Token string `json:"token,omitempty"`
}

// registeredSources is the closed source set, widened only by RegisterSource.
var registeredSources = map[string]bool{
	SourceMB: true, SourceMeet: true, SourceWeave: true, SourceSprint: true,
	SourceTodo: true, SourceInbox: true, SourcePing: true, SourceNotify: true,
	SourceActivity: true,
}

// RegisterSource admits a new subsystem to the contract. It exists so that
// adding a source is a one-line declaration at the adapter rather than an edit
// to this file — but it is still a declaration, so `bashy activity sources`
// can enumerate the surface instead of guessing it from traffic.
func RegisterSource(name string) error {
	name = strings.TrimSpace(name)
	if err := checkField("source", name, MaxFieldBytes); err != nil {
		return err
	}
	registeredSources[name] = true
	return nil
}

// Sources lists the registered sources, sorted.
func Sources() []string {
	out := make([]string, 0, len(registeredSources))
	for s := range registeredSources {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

var knownActions = map[string]bool{
	ActionCreate: true, ActionRead: true, ActionUpdate: true, ActionDelete: true,
	ActionStart: true, ActionFinish: true, ActionFail: true, ActionCancel: true,
	ActionBlock: true,
}

var knownStatus = map[string]bool{
	StatusOK: true, StatusFailed: true, StatusBlocked: true, StatusPending: true,
}

// Actions lists the closed action vocabulary, sorted.
func Actions() []string {
	out := make([]string, 0, len(knownActions))
	for a := range knownActions {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// Validate checks one caller-built event. Schema, ID, Seq and At are assigned
// by Emit and are not the caller's to set, so they are not checked here.
func (e Event) Validate() error {
	if !registeredSources[e.Source] {
		return fmt.Errorf("activity: unknown source %q (register it with RegisterSource; known: %s)",
			e.Source, strings.Join(Sources(), ", "))
	}
	if !knownActions[e.Action] {
		return fmt.Errorf("activity: unknown action %q (use one of: %s)", e.Action, strings.Join(Actions(), ", "))
	}
	if e.Status != "" && !knownStatus[e.Status] {
		return fmt.Errorf("activity: unknown status %q (use ok, failed, blocked or pending)", e.Status)
	}
	if strings.TrimSpace(e.Actor) == "" {
		return fmt.Errorf("activity: actor is required (an unattributed event cannot be excluded from its own author's inbox)")
	}
	if strings.TrimSpace(e.Noun) == "" {
		return fmt.Errorf("activity: noun is required")
	}
	if strings.TrimSpace(e.Object) == "" {
		return fmt.Errorf("activity: object reference is required (a recipient must be able to go look at the thing)")
	}
	if e.Hop < 0 || e.Hop >= MaxHop {
		return fmt.Errorf("activity: hop %d is at or past the limit of %d (refusing a delivery loop)", e.Hop, MaxHop)
	}
	fields := []struct {
		name, value string
		max         int
	}{
		{"actor", e.Actor, MaxActorBytes},
		{"noun", e.Noun, MaxFieldBytes},
		{"object", e.Object, MaxFieldBytes},
		{"owner", e.Owner, MaxActorBytes},
		{"scope.repo", e.Scope.Repo, MaxFieldBytes},
		{"scope.sprint", e.Scope.Sprint, MaxFieldBytes},
		{"scope.topic", e.Scope.Topic, MaxFieldBytes},
		{"scope.room", e.Scope.Room, MaxFieldBytes},
		{"cause", e.Cause, MaxFieldBytes},
		{"token", e.Token, MaxFieldBytes},
	}
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		if err := checkField(f.name, f.value, f.max); err != nil {
			return err
		}
	}
	lists := []struct {
		name   string
		values []string
		max    int
	}{
		{"mentions", e.Mentions, MaxActorBytes},
		{"assignees", e.Assignees, MaxActorBytes},
		{"depends_on", e.DependsOn, MaxFieldBytes},
		{"members", e.Members, MaxActorBytes},
	}
	for _, l := range lists {
		if len(l.values) > MaxListLen {
			return fmt.Errorf("activity: %s has %d entries; maximum is %d (refused, not truncated)", l.name, len(l.values), MaxListLen)
		}
		for _, v := range l.values {
			if err := checkField(l.name, v, l.max); err != nil {
				return err
			}
		}
	}
	return nil
}

// credentialPrefixes is defence in depth, not the primary control. The primary
// control is that there is no body field and every value is capped at 96 bytes,
// which is too small for a diff and too small for most terminal output. This
// catches the one thing that IS short enough to fit and must never be
// announced: a credential pasted into an object reference or a scope name.
var credentialPrefixes = []string{
	"sk-", "sk_live_", "sk_test_", "ghp_", "gho_", "ghu_", "ghs_", "ghr_",
	"github_pat_", "xoxb-", "xoxp-", "xoxa-", "AKIA", "ASIA", "AIza", "ya29.",
	"glpat-", "npm_", "dop_v1_", "hf_", "-----BEGIN",
}

func checkField(name, value string, max int) error {
	if len(value) > max {
		return fmt.Errorf("activity: %s is %d bytes; maximum is %d (refused, not truncated)", name, len(value), max)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("activity: %s contains a control character; the envelope carries references, never bodies or terminal output", name)
		}
	}
	for _, p := range credentialPrefixes {
		if strings.Contains(value, p) {
			return fmt.Errorf("activity: %s looks like a credential (%q); refusing to announce it", name, p)
		}
	}
	return nil
}

// computeID derives the stable, deterministic event id.
//
// Neither At nor Seq participates. Putting a clock in a key is the mistake
// docs/agentic-history-and-space-graph.md records the cost of: every emit
// becomes unique, the dedup index silently fills with n=1 singletons, and
// at-least-once quietly degrades to at-least-once-per-retry. What makes two
// emits the same event is that they describe the same actor doing the same
// thing to the same object at the same transaction boundary — which is exactly
// the tuple hashed here.
func (e Event) computeID() string {
	h := sha256.New()
	for _, f := range []string{
		e.Source, e.Actor, e.Action, e.Noun, e.Object,
		e.Scope.Repo, e.Scope.Sprint, e.Scope.Topic, e.Scope.Room,
		e.Token,
	} {
		h.Write([]byte(f))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:20]
}

// MaxSubjectBytes bounds the one-line rendering handed to the bus.
const MaxSubjectBytes = 256

// Subject renders the compact one-line form a recipient actually sees.
//
// It is built for a tiny token budget: verb-first, no punctuation ceremony,
// scope only when set. If the line would exceed MaxSubjectBytes the SCOPE
// fields are elided in a fixed order — never the id, and never the object.
// Eliding a rendering is safe because the rendering is a view: the complete
// envelope stays in the journal and `bashy activity show <id>` returns it, so
// the trailing id is the one field that must survive.
func (e Event) Subject() string {
	var b strings.Builder
	b.WriteString(e.Source)
	b.WriteString(" ")
	b.WriteString(e.Action)
	b.WriteString(" ")
	b.WriteString(e.Noun)
	b.WriteString(" ")
	b.WriteString(e.Object)
	if e.Status != "" {
		b.WriteString(" ")
		b.WriteString(e.Status)
	}
	head := b.String()
	tail := " id=" + e.ID
	// Scope fields, elided from the least selective to the most.
	scope := []struct{ k, v string }{
		{"room", e.Scope.Room},
		{"topic", e.Scope.Topic},
		{"sprint", e.Scope.Sprint},
		{"repo", e.Scope.Repo},
	}
	for drop := 0; drop <= len(scope); drop++ {
		var parts []string
		for i := len(scope) - 1; i >= drop; i-- {
			if scope[i].v != "" {
				parts = append(parts, scope[i].k+"="+scope[i].v)
			}
		}
		line := head
		if len(parts) > 0 {
			line += " [" + strings.Join(parts, " ") + "]"
		}
		line += tail
		if len(line) <= MaxSubjectBytes || drop == len(scope) {
			return line
		}
	}
	return head + tail
}
