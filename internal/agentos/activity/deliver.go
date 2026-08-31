// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package activity

// Delivery.
//
// The ordering here is the entire guarantee, so it is written once, in one
// function, rather than re-derived per subsystem:
//
//	1. journal the record as UNDELIVERED and fsync it       (the crash evidence)
//	2. ensure the recipient has a durable inbox              (offline recipients)
//	3. bus.Publish — the DURABLE APPEND                      (now it cannot be lost)
//	4. journal the per-recipient delivery                    (dedup on replay)
//	5. bus.SteerLive — the WAKE                              (best effort, always last)
//
// Step 5 is last and is allowed to fail. That is the point: a wake that fails
// costs latency, because the recipient reads the same event at its next turn
// boundary from the inbox step 3 already wrote. A wake that came FIRST, or a
// publish that was skipped because a wake succeeded, would cost the event.
// "No lost event between durable append and wake" is satisfied by the fact
// that the wake is strictly downstream of the append and never gates it.

import (
	"fmt"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
)

// CoalesceWindow is how long a wake about one object suppresses the next wake
// about the same object to the same recipient.
//
// Coalescing applies to the WAKE ONLY. Every routed event is still durably
// published, every time. Suppressing the durable copy would be the tidier
// implementation and it would silently drop the second half of a rapid
// create-then-fail pair, which is the half that mattered. DEMOTE, NEVER DROP —
// the same rule pkg/bus states for its own per-minute cap.
const CoalesceWindow = 10 * time.Second

// Now is the clock, injectable so the integration tests can assert on
// coalescing and rate limiting without sleeping.
var Now = time.Now

// Transport seams. These are function variables for the same reason
// bus.SteerFrame and bus.FleetNames are: a test must be able to observe a
// delivery without a live agent on the other end, and — load-bearing here —
// without touching the operator's real bus, room timeline or inbox.
var (
	// EnsureInbox gives an offline recipient a durable inbox before anything is
	// addressed to it. Without this a notification to an agent that never ran
	// `bus subscribe` matches no stored subscription and reaches nobody while
	// looking sent.
	EnsureInbox = func(subscriber string) error {
		_, err := bus.EnsureSubscription(subscriber)
		return err
	}

	// PublishDurable appends to the append-only room timeline. This is
	// bashy#10's primitive and the ONLY mailbox in this design.
	PublishDurable = func(principal, to, subject, topic, priority string) error {
		return bus.Publish(bus.Notification{
			Principal: principal, To: to, Body: subject, Topic: topic, Priority: priority,
		})
	}

	// WakeLive pushes into a live agent conversation over the existing session
	// control path. It reports whether the frame landed and, if not, why.
	WakeLive = func(subscriber, subject string) (bool, string) {
		d := bus.SteerLive(subscriber, subject)
		return d.Steered, d.Reason
	}
)

// TopicFor is the bus topic an activity event publishes under. Keeping every
// activity event under a dotted `activity.<source>` topic means an operator can
// subscribe to the raw stream with the bus tooling that already exists
// (`bashy bus subscribe --topic activity.weave`) without this package growing a
// second subscription surface for the same idea.
func TopicFor(source string) string { return "activity." + source }

// Result is what one Emit did, and is the adapter API's return value.
type Result struct {
	Event      Event       `json:"event"`
	Recipients []Recipient `json:"recipients,omitempty"`
	// Duplicate is true when this exact event id was already fully delivered.
	// The emit is then a no-op, which is what makes an adapter safe to call on
	// every recovery path.
	Duplicate bool `json:"duplicate,omitempty"`
	// Wakes maps subscriber to the wake outcome (steered, queued, coalesced,
	// rate-limited, unreachable).
	Wakes map[string]string `json:"wakes,omitempty"`
	// Errors are per-recipient delivery failures. A failure for one recipient
	// never abandons the others: one unreachable agent must not silence the
	// rest of the fleet.
	Errors []string `json:"errors,omitempty"`
}

// Emit is THE adapter API. A subsystem calls it exactly once, at a SUCCESSFUL
// transaction boundary — after its own write has committed, never before.
//
// Emitting before the commit would announce a fact that may not become true,
// and an agent woken to look at a run that does not exist learns to distrust
// the channel. Emitting on failure of the subsystem's own write is likewise
// wrong: there is no object to point at.
//
// It is idempotent on the event id, so calling it again after a crash, a
// retry, or a queue replay delivers nothing twice.
func Emit(e Event) (Result, error) {
	if e.Source == SourceActivity {
		return Result{}, fmt.Errorf("activity: %q is a reserved source; the delivery path must not announce itself (loop prevention)", SourceActivity)
	}
	if err := e.Validate(); err != nil {
		return Result{}, err
	}
	e.Schema = SchemaVersion
	e.ID = e.computeID()

	s, err := openStore()
	if err != nil {
		return Result{}, err
	}
	l, err := s.lock("emit " + e.Source)
	if err != nil {
		return Result{}, err
	}
	defer l.Release()

	records, err := s.load()
	if err != nil {
		return Result{}, err
	}

	if prior, ok := findRecord(records, e.ID); ok && prior.Published {
		return Result{Event: prior.Event, Recipients: prior.Recipients, Duplicate: true, Wakes: prior.Wakes}, nil
	}

	rec, resumed := findRecord(records, e.ID)
	if resumed {
		// A partially delivered record from a previous process. Keep its
		// sequence and its routing decision: re-routing on recovery could
		// produce a different recipient set from a changed interest file, and
		// then half the fleet would hold an event the other half was never
		// told about.
		e = rec.Event
	} else {
		e.At = Now().UTC()
		e.Seq = nextSeq(records, e.Source)
		rec = Record{Event: e, Recipients: Route(e, mustInterests(s)), Wakes: map[string]string{}}
		if err := s.append(rec); err != nil {
			return Result{}, err
		}
	}
	res := deliver(s, records, &rec)
	if err := s.append(rec); err != nil {
		return res, err
	}
	if err := s.prune(append(records, rec)); err != nil {
		// Pruning is housekeeping. A failure here must not turn a completed
		// delivery into a reported failure.
		res.Errors = append(res.Errors, "prune: "+err.Error())
	}
	return res, nil
}

// mustInterests reads the interest file, treating an unreadable one as empty.
// The alternative — failing the emit — would let a corrupt preferences file
// stop the whole activity plane, and an interest file is preferences.
func mustInterests(s *store) []Interest {
	in, err := s.loadInterests()
	if err != nil {
		return nil
	}
	return in
}

// deliver runs steps 2–5 for every recipient not already delivered.
func deliver(s *store, history []Record, rec *Record) Result {
	res := Result{Event: rec.Event, Recipients: rec.Recipients, Wakes: map[string]string{}}
	// Both maps are initialized HERE rather than at the call sites. deliver is
	// reached from Emit and from Recover, and a record that round-tripped
	// through the journal comes back with nil maps (both fields are
	// omitempty, so an empty one is not written at all). Initializing in Emit
	// only is the version of this that passes every test until the first
	// crash-recovery path runs, which is exactly when it must not panic.
	if rec.Delivered == nil {
		rec.Delivered = map[string]bool{}
	}
	if rec.Wakes == nil {
		rec.Wakes = map[string]string{}
	}
	if len(rec.Recipients) == 0 {
		// Routed to nobody. This is a normal outcome, not a failure: it is what
		// "no broad broadcast" looks like from the inside. The record is still
		// marked published so the id is deduped and the event stays visible to
		// `bashy activity tail`, which is how an operator sees that an event was
		// emitted and interested nobody.
		rec.Published = true
		return res
	}

	subject := rec.Event.Subject()
	topic := TopicFor(rec.Event.Source)
	allDone := true

	for _, r := range rec.Recipients {
		if rec.Delivered[r.Subscriber] {
			if outcome, ok := rec.Wakes[r.Subscriber]; ok {
				res.Wakes[r.Subscriber] = outcome
			}
			continue
		}
		if err := EnsureInbox(r.Subscriber); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: inbox: %v", r.Subscriber, err))
			allDone = false
			continue
		}
		// Step 3 — the durable append. Priority mirrors the routing decision so
		// the bus's own tiering agrees with ours rather than contradicting it.
		priority := bus.DeliveryQueued
		if r.Wake {
			priority = bus.DeliveryInterrupt
		}
		if err := PublishDurable(rec.Event.Source, r.Subscriber, subject, topic, priority); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: publish: %v", r.Subscriber, err))
			allDone = false
			continue
		}
		rec.Delivered[r.Subscriber] = true

		// Step 5 — the wake, strictly after the durable append.
		outcome := WakeQueued
		if r.Wake {
			outcome = wakeOutcome(s, history, rec.Event, r, subject)
		}
		rec.Wakes[r.Subscriber] = outcome
		res.Wakes[r.Subscriber] = outcome
	}
	rec.Published = allDone
	if !allDone {
		rec.Note = "partial delivery; recover will re-drive the remainder"
	} else {
		rec.Note = ""
	}
	return res
}

// wakeOutcome applies backpressure, then wakes.
//
// Both controls DEMOTE to queued rather than dropping, so the recipient still
// reads the event at its next turn boundary. The only thing lost is the
// interruption, which is the thing that was too frequent.
func wakeOutcome(s *store, history []Record, e Event, r Recipient, subject string) string {
	interests := mustInterests(s)
	var in Interest
	for _, cand := range interests {
		if strings.EqualFold(cand.Subscriber, r.Subscriber) {
			in = cand
			break
		}
	}
	now := Now().UTC()

	// Coalescing: one wake per (recipient, source, object) per window.
	for _, prior := range history {
		if prior.Wakes[r.Subscriber] != WakeSteered {
			continue
		}
		if prior.Event.Source != e.Source || prior.Event.Object != e.Object {
			continue
		}
		if now.Sub(prior.Event.At) < CoalesceWindow {
			return WakeCoalesced
		}
	}

	// Rate limiting: a bounded number of wakes per minute per recipient.
	cap := DefaultMaxWakePerMin
	if in.Subscriber != "" {
		cap = in.maxWakePerMin()
	}
	woken := 0
	for _, prior := range history {
		if prior.Wakes[r.Subscriber] != WakeSteered {
			continue
		}
		if now.Sub(prior.Event.At) < time.Minute {
			woken++
		}
	}
	if woken >= cap {
		return WakeRateLimited
	}

	if steered, _ := WakeLive(r.Subscriber, subject); steered {
		return WakeSteered
	}
	return WakeUnreachable
}

// Recover re-drives every record the journal shows as owed.
//
// This is the crash-recovery half of at-least-once. It is safe to run at any
// time and on every start: a fully delivered record is skipped, and a
// partially delivered one resumes at the recipient it stopped on rather than
// re-publishing to the ones already served.
func Recover() ([]Result, error) {
	s, err := openStore()
	if err != nil {
		return nil, err
	}
	l, err := s.lock("recover")
	if err != nil {
		return nil, err
	}
	defer l.Release()

	records, err := s.load()
	if err != nil {
		return nil, err
	}
	var out []Result
	for i := range records {
		if records[i].Published {
			continue
		}
		rec := records[i]
		res := deliver(s, records, &rec)
		if err := s.append(rec); err != nil {
			return out, err
		}
		out = append(out, res)
	}
	return out, nil
}

// Tail returns the most recent records, newest last.
//
// This is the COMPATIBILITY AND RECOVERY fallback the design allows, not the
// normal read path. Normal operation is push: the durable publish lands in the
// recipient's bus inbox and `bashy inbox` renders it. An agent that polls Tail
// on a timer has reintroduced exactly the token-heavy polling this contract
// exists to remove — it is here for an operator diagnosing delivery and for a
// subscriber reconnecting after a gap it cannot close from its own cursor.
func Tail(limit int, source string) ([]Record, error) {
	s, err := openStore()
	if err != nil {
		return nil, err
	}
	records, err := s.load()
	if err != nil {
		return nil, err
	}
	if source != "" {
		var filtered []Record
		for _, r := range records {
			if r.Event.Source == source {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}
	if limit > 0 && len(records) > limit {
		records = records[len(records)-limit:]
	}
	return records, nil
}

// Since returns records for one source after a sequence — the reconnect
// catch-up query. It answers "what did I miss on this source", which a
// subscriber can ask once on reconnect instead of polling.
func Since(source string, seq int64) ([]Record, error) {
	s, err := openStore()
	if err != nil {
		return nil, err
	}
	records, err := s.load()
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, r := range records {
		if r.Event.Source == source && r.Event.Seq > seq {
			out = append(out, r)
		}
	}
	return out, nil
}

// Show returns one record by event id — how a recipient turns the trailing
// `id=` in a rendered subject back into the complete envelope.
func Show(id string) (Record, bool, error) {
	s, err := openStore()
	if err != nil {
		return Record{}, false, err
	}
	records, err := s.load()
	if err != nil {
		return Record{}, false, err
	}
	r, ok := findRecord(records, id)
	return r, ok, nil
}
