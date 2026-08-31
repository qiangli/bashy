// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package activity

// The ADAPTER API — the surface a subsystem wires itself to this contract
// with, and the surface bashy#11 consumes.
//
// The design constraint is that wiring a subsystem must be a ONE-LINE call at
// its transaction boundary. Anything longer and the wiring drifts per
// subsystem, which is the exact failure this package was written to end: three
// hand-rolled notice types that each made the same five decisions slightly
// differently. So the adapter carries the parts that are constant for a
// subsystem (its source name, its actor, its scope) and each call site
// supplies only what is genuinely per-event: the action, the noun, the object
// reference, the transaction token, and who is connected to it.

import (
	"fmt"
	"strings"
)

// Adapter binds a subsystem's constant identity.
type Adapter struct {
	source string
	actor  string
	scope  Scope
}

// For returns the adapter for a source, refusing an unregistered one.
//
// Refusing is the point. A typo'd source would otherwise produce events that
// route (Route does not care about the spelling) but that no interest can
// name and `bashy activity sources` cannot list — a working delivery path that
// is undiscoverable, which is the worst kind because it looks fine.
func For(source string) (*Adapter, error) {
	source = strings.TrimSpace(source)
	if source == SourceActivity {
		return nil, fmt.Errorf("activity: %q is reserved for the delivery path itself", SourceActivity)
	}
	if !registeredSources[source] {
		return nil, fmt.Errorf("activity: %q is not a registered source (known: %s)", source, strings.Join(Sources(), ", "))
	}
	return &Adapter{source: source}, nil
}

// As sets the actor for events from this adapter. The actor is who DID the
// thing, and it is required: Route excludes an event's own actor from its
// recipients, so an unattributed event is one that can echo back to its author.
func (a *Adapter) As(actor string) *Adapter {
	b := *a
	b.actor = strings.TrimSpace(actor)
	return &b
}

// In sets the default scope. Returns a copy, so an adapter shared across
// goroutines cannot have its scope changed underneath a call in flight.
func (a *Adapter) In(s Scope) *Adapter {
	b := *a
	b.scope = s
	return &b
}

// Source reports the bound source name.
func (a *Adapter) Source() string { return a.source }

// Interested bundles the routing inputs — the people and objects connected to
// one event. Every field is optional; an event with none of them routes only
// to whatever standing interests match its scope, which is the correct floor
// for a change that concerns nobody in particular.
type Interested struct {
	// Mentions are identities this event names directly. The strongest reason.
	Mentions []string
	// Owner is the single accountable identity for the object.
	Owner string
	// Assignees are the identities doing the work on the object.
	Assignees []string
	// DependsOn are OBJECT references this object depends on, not identities.
	// A subscriber watching one of them matches by dependency.
	DependsOn []string
	// Members are the identities in the event's scope — the sprint's agents,
	// the room's participants. The broadest reason, and the one that most needs
	// to be a deliberate list rather than "everybody".
	Members []string
}

// Announce is the one-line call site. It emits at a SUCCESSFUL transaction
// boundary and returns what was delivered and to whom.
//
// token is what makes this commit of this object distinct from the next one —
// a terminal state, a revision number, a monotonic id. Two calls with the same
// token are the same event and the second is a no-op, which is what makes this
// safe to call from a recovery path that cannot know whether it already ran.
//
// A caller that cannot supply a meaningful token should pass the object's new
// state. A caller that passes a clock has defeated the dedup: every emit
// becomes unique and at-least-once degrades to once-per-retry.
func (a *Adapter) Announce(action, noun, object, status, token string, who Interested) (Result, error) {
	if a == nil {
		return Result{}, fmt.Errorf("activity: nil adapter")
	}
	return Emit(Event{
		Source: a.source,
		Actor:  a.actor,
		Action: action,
		Noun:   noun,
		Object: object,
		Scope:  a.scope,
		Status: status,
		Token:  token,

		Mentions:  who.Mentions,
		Owner:     who.Owner,
		Assignees: who.Assignees,
		DependsOn: who.DependsOn,
		Members:   who.Members,
	})
}

// Created, Updated and Deleted are the CRUD shorthands. Status is ok, because
// a create that failed did not create anything and has no object to point at —
// use Lifecycle with ActionFail for that.
func (a *Adapter) Created(noun, object, token string, who Interested) (Result, error) {
	return a.Announce(ActionCreate, noun, object, StatusOK, token, who)
}

func (a *Adapter) Updated(noun, object, token string, who Interested) (Result, error) {
	return a.Announce(ActionUpdate, noun, object, StatusOK, token, who)
}

func (a *Adapter) Deleted(noun, object, token string, who Interested) (Result, error) {
	return a.Announce(ActionDelete, noun, object, StatusOK, token, who)
}

// Read emits a read event.
//
// It is a separate method rather than a fourth CRUD shorthand so that the
// asymmetry is visible at the call site: a read is NEVER broad-broadcast. It
// reaches only an identity that asked for read activity by name (Interest.Audit
// or an explicit "read" action), and it never routes on membership. On a host
// with no audit interest declared, this call is a durable journal entry and
// nothing else — which is the intended behaviour, not a misconfiguration.
func (a *Adapter) Read(noun, object, token string, who Interested) (Result, error) {
	return a.Announce(ActionRead, noun, object, StatusOK, token, who)
}

// Lifecycle emits a start/finish/fail/cancel/block event. status is the
// minimal outcome; for ActionFail it should be StatusFailed, and the object
// reference is how a recipient finds out what actually went wrong. The failure
// TEXT deliberately has nowhere to go in this envelope: terminal output is the
// single largest thing an activity event must never carry.
func (a *Adapter) Lifecycle(action, noun, object, status, token string, who Interested) (Result, error) {
	return a.Announce(action, noun, object, status, token, who)
}

// Reacting builds an event caused by another one, carrying the loop-prevention
// chain forward. A subsystem that emits activity in RESPONSE to activity must
// use this: Emit refuses at MaxHop, so a cycle terminates in a reported error
// at a named call site rather than in an unbounded fan-out.
func (a *Adapter) Reacting(cause Event, action, noun, object, status, token string, who Interested) (Result, error) {
	return Emit(Event{
		Source: a.source, Actor: a.actor, Action: action, Noun: noun,
		Object: object, Scope: a.scope, Status: status, Token: token,
		Mentions: who.Mentions, Owner: who.Owner, Assignees: who.Assignees,
		DependsOn: who.DependsOn, Members: who.Members,
		Cause: cause.ID, Hop: cause.Hop + 1,
	})
}
