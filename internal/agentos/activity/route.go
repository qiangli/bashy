// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package activity

// The interest-routing matrix.
//
// Routing is the whole reason this contract exists. Broadcasting every event
// to every agent is cheap to implement and ruinous in practice: an agent that
// is interrupted for something irrelevant is worse off than one left alone,
// and an inbox that is 90% noise gets skimmed, which is the same as not
// delivering the 10%. So an event reaches an identity only when a NAMED
// relationship connects them, and the relationship is reported alongside the
// delivery — a recipient who cannot see why it was told something cannot
// decide whether the sender was wrong.

import (
	"fmt"
	"sort"
	"strings"
)

// Match reasons, in the precedence order this package resolves ties by.
//
// The order is DIRECTNESS OF THIS EVENT TO THIS RECIPIENT, not how the
// relationship was established. Being named in the event outranks a standing
// subscription because the standing subscription says "tell me about things
// like this" while the mention says "this one is about you" — and when both
// are true the second is the honest explanation. Membership is last because
// it is the only reason that is true of a whole group rather than a person.
const (
	ReasonMention      = "mention"
	ReasonAssignment   = "assignment"
	ReasonOwnership    = "ownership"
	ReasonSubscription = "subscription"
	ReasonDependency   = "dependency"
	ReasonMembership   = "membership"
)

var reasonPrecedence = []string{
	ReasonMention,
	ReasonAssignment,
	ReasonOwnership,
	ReasonSubscription,
	ReasonDependency,
	ReasonMembership,
}

func reasonRank(r string) int {
	for i, want := range reasonPrecedence {
		if want == r {
			return i
		}
	}
	return len(reasonPrecedence)
}

// ReasonPrecedence returns the reason vocabulary, strongest first.
func ReasonPrecedence() []string { return append([]string(nil), reasonPrecedence...) }

// wakeReasons are the reasons that justify breaking into a live turn. The
// split is the report/author split the fleet uses everywhere else: being named,
// assigned or accountable is a claim on your attention now; a standing interest
// is a claim on your attention at the next turn boundary.
var wakeReasons = map[string]bool{
	ReasonMention:    true,
	ReasonAssignment: true,
	ReasonOwnership:  true,
}

// Interest is one identity's standing declaration of what it wants to hear
// about. It is deliberately NOT bus.Subscription: that type governs the
// transport (who may interrupt me, how many per minute, where my cursor is)
// and this one governs the contract (which activity is mine). Folding them
// together would mean an agent could not gain an inbox without also gaining
// the firehose, which is precisely the default bus.EnsureSubscription refuses
// to set.
type Interest struct {
	Subscriber string `json:"subscriber"`

	// Sources limits which subsystems reach this subscriber. Empty means every
	// registered source; "*" is accepted as the explicit spelling of the same.
	Sources []string `json:"sources,omitempty"`
	// Nouns limits which object kinds reach it. Empty means every noun.
	Nouns []string `json:"nouns,omitempty"`
	// Actions limits which actions reach it. Empty means every action EXCEPT
	// read — see Audit.
	Actions []string `json:"actions,omitempty"`

	// Repos, Sprints and Topics are the scope axis. A match on any of them is a
	// ReasonSubscription match.
	Repos   []string `json:"repos,omitempty"`
	Sprints []string `json:"sprints,omitempty"`
	Topics  []string `json:"topics,omitempty"`

	// Objects are specific object references this subscriber watches. A watched
	// object appearing as the event's own Object is a subscription; appearing in
	// the event's DependsOn is a dependency.
	Objects []string `json:"objects,omitempty"`

	// Audit opts in to read events. Read activity is never broad-broadcast, so
	// without this (or an explicit "read" in Actions) a read event routes to
	// nobody at all.
	Audit bool `json:"audit,omitempty"`

	// Wake false keeps every delivery at the queued tier for this subscriber,
	// whatever the reason. An operator who has decided this agent is never to be
	// interrupted has expressed a policy, and a strong match must not override
	// it.
	Wake bool `json:"wake"`

	// MaxWakePerMin caps wakes per minute before the surplus is DEMOTED to
	// queued. Zero means DefaultMaxWakePerMin. Nothing is ever dropped.
	MaxWakePerMin int `json:"max_wake_per_min,omitempty"`

	// Mute suppresses routing entirely. This is the one place an event is
	// deliberately not delivered, and it is an explicit instruction from the
	// subscriber rather than an inference by the router.
	Mute bool `json:"mute,omitempty"`
}

// DefaultMaxWakePerMin mirrors bus.DefaultMaxPerMin's reasoning and its value:
// steering quality collapses as interruptions multiply, so the cap is low and
// deliberately unimpressive. Exceeding it never drops anything.
const DefaultMaxWakePerMin = 3

func (i Interest) maxWakePerMin() int {
	if i.MaxWakePerMin > 0 {
		return i.MaxWakePerMin
	}
	return DefaultMaxWakePerMin
}

// Recipient is one routed delivery and the justification for it.
type Recipient struct {
	Subscriber string `json:"subscriber"`
	Reason     string `json:"reason"`
	// Why is the human- and agent-readable explanation: which field of which
	// event connected this identity to it. `bashy activity routes` prints it,
	// and it is what makes a wrong subscription diagnosable instead of
	// mysterious.
	Why  string `json:"why"`
	Wake bool   `json:"wake"`
}

// Route resolves an event to its recipients, strongest reason first, then by
// subscriber name. The result is deterministic: same event and same interests
// give the same slice in the same order, on any host, which is what lets the
// integration tests assert on it and what lets two agents agree about who was
// told.
func Route(e Event, interests []Interest) []Recipient {
	// Loop-prevention floor. Announcing the delivery machinery's own traffic as
	// activity is a cycle with no fixed point.
	if e.Source == SourceActivity {
		return nil
	}
	if e.Hop >= MaxHop {
		return nil
	}

	byName := make(map[string]Interest, len(interests))
	for _, i := range interests {
		if name := strings.TrimSpace(i.Subscriber); name != "" {
			byName[name] = i
		}
	}

	best := map[string]Recipient{}
	offer := func(name, reason, why string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		// Never route an event back to the identity that caused it. An agent
		// does not need to be told what it just did, and an actor-echo is the
		// cheapest way to build an accidental loop out of two subsystems that
		// each react to the other.
		if strings.EqualFold(name, strings.TrimSpace(e.Actor)) {
			return
		}
		in, subscribed := byName[name]
		if subscribed && in.Mute {
			return
		}
		// A read event is never broad-broadcast. It reaches only an identity
		// that asked for read activity by name — an audit interest, or one that
		// listed "read" explicitly — and never on a membership reason, which is
		// the broadcast axis wearing a different hat.
		if e.Action == ActionRead {
			if !subscribed || !(in.Audit || stringIn(in.Actions, ActionRead)) {
				return
			}
			if reason == ReasonMembership {
				return
			}
		}
		// A subscriber's declared filters bind on every reason, not only on the
		// subscription reason: an interest that says "only weave" means it, even
		// when the match came from being mentioned.
		if subscribed && !in.admits(e) {
			return
		}
		wake := wakeReasons[reason]
		if subscribed && !in.Wake {
			wake = false
		}
		cand := Recipient{Subscriber: name, Reason: reason, Why: why, Wake: wake}
		prev, seen := best[name]
		if !seen || reasonRank(reason) < reasonRank(prev.Reason) {
			// A stronger reason also carries the stronger wake tier.
			best[name] = cand
			return
		}
		// Equal or weaker reason: keep the recorded explanation, but never
		// downgrade a wake that a stronger reason already justified.
		if seen && cand.Wake && !prev.Wake && reasonRank(reason) == reasonRank(prev.Reason) {
			prev.Wake = true
			best[name] = prev
		}
	}

	for _, m := range e.Mentions {
		offer(m, ReasonMention, fmt.Sprintf("mentioned in %s", e.Object))
	}
	for _, a := range e.Assignees {
		offer(a, ReasonAssignment, fmt.Sprintf("assigned to %s", e.Object))
	}
	offer(e.Owner, ReasonOwnership, fmt.Sprintf("owns %s", e.Object))

	// Explicit standing interest.
	for _, in := range interests {
		if why, ok := in.subscriptionMatch(e); ok {
			offer(in.Subscriber, ReasonSubscription, why)
		}
	}
	// Dependency: this subscriber watches an object that the event's object
	// depends on. Derived, so it ranks below the interest that was declared.
	for _, in := range interests {
		for _, dep := range e.DependsOn {
			if stringIn(in.Objects, dep) {
				offer(in.Subscriber, ReasonDependency, fmt.Sprintf("%s depends on watched %s", e.Object, dep))
				break
			}
		}
	}
	// Membership in the event's scope. The broadest reason, and the only one
	// that is true of a group.
	for _, m := range e.Members {
		offer(m, ReasonMembership, fmt.Sprintf("member of %s", scopeLabel(e.Scope)))
	}

	out := make([]Recipient, 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	sort.Slice(out, func(a, b int) bool {
		ra, rb := reasonRank(out[a].Reason), reasonRank(out[b].Reason)
		if ra != rb {
			return ra < rb
		}
		return out[a].Subscriber < out[b].Subscriber
	})
	return out
}

// admits applies the declared filters that bind regardless of match reason.
func (i Interest) admits(e Event) bool {
	if len(i.Sources) > 0 && !stringIn(i.Sources, e.Source) && !stringIn(i.Sources, "*") {
		return false
	}
	if len(i.Nouns) > 0 && !stringIn(i.Nouns, e.Noun) && !stringIn(i.Nouns, "*") {
		return false
	}
	if len(i.Actions) > 0 && !stringIn(i.Actions, e.Action) && !stringIn(i.Actions, "*") {
		return false
	}
	return true
}

// subscriptionMatch reports whether a declared interest covers this event on
// its own — the scope and object axes. Sources/Nouns/Actions are FILTERS, not
// matchers: an interest that named only a source is asking to hear that
// source's events, so it must match on its own.
func (i Interest) subscriptionMatch(e Event) (string, bool) {
	if i.Mute {
		return "", false
	}
	if !i.admits(e) {
		return "", false
	}
	if stringIn(i.Objects, e.Object) {
		return fmt.Sprintf("watching object %s", e.Object), true
	}
	if e.Scope.Repo != "" && stringIn(i.Repos, e.Scope.Repo) {
		return fmt.Sprintf("subscribed to repo %s", e.Scope.Repo), true
	}
	if e.Scope.Sprint != "" && stringIn(i.Sprints, e.Scope.Sprint) {
		return fmt.Sprintf("subscribed to sprint %s", e.Scope.Sprint), true
	}
	if e.Scope.Topic != "" && stringIn(i.Topics, e.Scope.Topic) {
		return fmt.Sprintf("subscribed to topic %s", e.Scope.Topic), true
	}
	// An interest that declared a source or noun filter and no scope at all is
	// asking for that whole stream. This is the only unscoped match, and it is
	// explicit by construction: an empty Interest matches nothing here.
	if len(i.Repos) == 0 && len(i.Sprints) == 0 && len(i.Topics) == 0 && len(i.Objects) == 0 {
		if len(i.Sources) > 0 {
			return fmt.Sprintf("subscribed to source %s", e.Source), true
		}
		if len(i.Nouns) > 0 {
			return fmt.Sprintf("subscribed to noun %s", e.Noun), true
		}
	}
	return "", false
}

func scopeLabel(s Scope) string {
	var parts []string
	if s.Repo != "" {
		parts = append(parts, "repo "+s.Repo)
	}
	if s.Sprint != "" {
		parts = append(parts, "sprint "+s.Sprint)
	}
	if s.Topic != "" {
		parts = append(parts, "topic "+s.Topic)
	}
	if s.Room != "" {
		parts = append(parts, "room "+s.Room)
	}
	if len(parts) == 0 {
		return "this event's scope"
	}
	return strings.Join(parts, ", ")
}

func stringIn(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
