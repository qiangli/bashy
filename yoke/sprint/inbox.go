package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/meet"
	"github.com/qiangli/yoke/pkg/policy/coord"
	"github.com/qiangli/yoke/pkg/principal"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/weave"
)

const unifiedInboxSchema = "bashy-inbox-v1"

const inboxWatcherMode = "inbox"

var inboxMeetRooms = meet.Rooms

type unifiedInboxEvent struct {
	Schema        string              `json:"schema"`
	Source        string              `json:"source"`
	Seq           int64               `json:"seq"`
	At            string              `json:"at,omitempty"`
	From          string              `json:"from,omitempty"`
	FromParty     *bus.Party          `json:"from_party,omitempty"`
	FromHandle    string              `json:"from_handle,omitempty"`
	To            string              `json:"to,omitempty"`
	Topic         string              `json:"topic,omitempty"`
	Room          string              `json:"room,omitempty"`
	EventType     string              `json:"event_type,omitempty"`
	StoryID       string              `json:"story_id,omitempty"`
	SuggestedVerb string              `json:"suggested_verb,omitempty"`
	Body          string              `json:"body"`
	HuntGroup     bool                `json:"-"`
	Origin        *unifiedInboxOrigin `json:"origin,omitempty"`
}

type unifiedInboxOrigin struct {
	Source string `json:"source"`
	Seq    int64  `json:"seq"`
}

type inboxBatch struct {
	events []unifiedInboxEvent
	// hintIncomplete means the bounded per-command MB probe may have older
	// unread records. Explicit inbox reads never set this flag.
	hintIncomplete bool
	// before provenance collapse, for exact turn acknowledgements of seeded
	// Meet copies that share one rendered MB record.
	rawEvents  []unifiedInboxEvent
	roleTopics map[string]string
	acks       []func() error
	warns      []string
}

// inboxWatcherClaim is one registered watcher's hold on a fleet identity: how
// it is released, and the condition under which it is still legitimate. The two
// belong together — a claim whose owning session is gone must be released, and
// the loop that discovers that is the one holding it.
type inboxWatcherClaim struct {
	leave func()
	// ownerLive returns an error once the registered owner is provably no
	// longer this process's parent or ancestor. It is nil-safe to call
	// repeatedly and fails open where the process tree cannot be read.
	ownerLive func() error
}

func registerSprintInboxWatcher(reader string) (inboxWatcherClaim, error) {
	return registerInboxWatcherAs(reader, "sprint-inbox", "managing sprint with attached inbox stream",
		[]string{room.CapInboxStream})
}

func registerInboxWatcherAs(reader, mode, task string, caps []string) (inboxWatcherClaim, error) {
	id, err := resolveInboxWatcherIdentity(reader)
	if err != nil {
		return inboxWatcherClaim{}, err
	}

	// The attached-mode coord claim is the watcher lease. It closes the read-before-write
	// race between two fresh processes and also refuses two watcher loops in
	// one process, since both would otherwise advance the same source cursors.
	// Keyed by owner identity name under kind "inbox".
	//
	// BASHY_CLAIM=0|off bypasses the ledger: a watcher must still start when the
	// advisory ledger is unavailable or switched off. Exclusivity then rests on
	// the in-process table (two loops in one process) and room.Join (two
	// processes), which is what the lease only duplicated.
	var (
		grant coord.Grant
		lock  *lockfile.Lock
	)
	if claimDisabled() {
		if _, held := localInboxWatchers.LoadOrStore(id.claimID, struct{}{}); held {
			return inboxWatcherClaim{}, errInboxWatcherLive(id)
		}
	} else {
		var err error
		grant, lock, err = coord.AcquireAttachedRef(context.Background(), coord.Request{
			Ref:    coord.Ref{Kind: "inbox", Name: id.name},
			Holder: principal.Ref{Name: id.name},
			Intent: task,
			Mode:   coord.ModeAttached,
		})
		if err != nil {
			var conf *coord.Conflict
			if errors.As(err, &conf) || errors.Is(err, lockfile.ErrHeld) {
				return inboxWatcherClaim{}, errInboxWatcherLive(id)
			}
			return inboxWatcherClaim{}, fmt.Errorf("inbox: claim watcher identity %q: %w", id.name, err)
		}
	}
	releaseLease := func() {
		if lock == nil {
			localInboxWatchers.Delete(id.claimID)
			return
		}
		_ = coord.ReleaseAttached(coord.DefaultDir(), grant.Claim, lock)
	}
	cwd, _ := os.Getwd()
	ownerPID := os.Getppid()
	if ownerPID <= 1 {
		// PID 1 is a common ancestor, never a session proof. Leave OwnerPID empty
		// so the shared guard falls back to this watcher's live PID and fails
		// closed for sibling commands when no stronger tool-session claim exists.
		ownerPID = 0
	}
	card := room.Card{
		// The registered name is the global identity claim. A parallel
		// "inbox:NAME" card would let one identity occupy two live sessions.
		ID:        id.claimID,
		Principal: id.principal,
		// SessionClaim is the AGENT-to-AGENT impersonation proof; it is empty for
		// a person, whose OS login is the trust boundary (bus.ResolveAuthoredActor
		// decides authored mail the same way). Tool/Model/Binding likewise stay
		// empty for a person, who has no runnable agent binding to report.
		SessionClaim: id.sessionClaim,
		Tool:         id.tool,
		Model:        id.model,
		Binding:      id.binding,
		Nick:         id.nick,
		Mode:         mode,
		Task:         task,
		Caps:         caps,
		PID:          os.Getpid(),
		OwnerPID:     ownerPID,
		Cwd:          cwd,
	}
	if err := room.Join(card); err != nil {
		var live *room.ErrLive
		if mode == inboxWatcherMode && lock != nil && errors.As(err, &live) {
			// The identity is live under a card this watch did not write.
			// A managed session holds its own name exactly this way, so a
			// watch running inside that same session may share it. Anything
			// else falls through to the refusal below, unchanged.
			if adopted, ok := adoptManagedSessionWatch(id, releaseLease); ok {
				return adopted, nil
			}
		}
		releaseLease()
		if lock == nil && errors.As(err, &live) {
			return inboxWatcherClaim{}, errInboxWatcherLive(id)
		}
		return inboxWatcherClaim{}, fmt.Errorf("inbox: register watcher %q: %w", id.name, err)
	}
	anchor := inboxWatcherAnchor(card)
	return inboxWatcherClaim{
		leave: func() {
			// Both halves, always. The room card is what `bashy agent` and the
			// authored-identity guard read; the kernel claim is what the next
			// watcher process must take. Releasing one and keeping the other
			// leaves the identity half-held, which reads as available in one
			// surface and taken in the other.
			retireInboxWatcherCard(card)
			releaseLease()
		},
		ownerLive: func() error {
			if inboxOwnerRelation(os.Getpid(), anchor) == inboxOwnerGone {
				return &inboxOwnerGoneError{agent: id.name, owner: anchor}
			}
			return nil
		},
	}, nil
}

// adoptManagedSessionWatch shares a managed session's own room identity with
// an inbox watch running inside that same session, instead of refusing it as
// a duplicate owner.
//
// A managed (foreman) session holds its singleton name card for its whole
// lifetime, so `inbox --watch` could never register and no supported command
// could drain that identity's overflow: bounded reads peek by design and
// `sprint inbox-ack` needs an attached sprint watch the foreman is not.
//
// The adoption proves the session instead of taking the identity: the live
// card must be this name's foreman-mode card, and this process must descend
// from its holder (the inboxOwnerRelation proof — liveness alone is not
// identity, a recycled pid must not satisfy it). Nothing is written to the
// room: there is still exactly one card, still held by the foreman, so leave
// releases only the kernel lease the watch acquired and never retires the
// card. Single-consumer exclusion is unchanged — the caller holds the
// exclusive attached lease, so a second concurrent watch is refused before it
// gets here — and the claim is offered only while that lease is held
// (BASHY_CLAIM=0 keeps the old refusal). Unknown ancestry fails closed.
//
// The adopted watch drains exactly like any other: the loop renders each
// batch before its per-source watermarks advance, so no undisplayed record is
// ever acknowledged, and the ownerLive guard below stops the watch (without
// consuming) once the owning session is gone.
func adoptManagedSessionWatch(id inboxWatcherIdentity, releaseLease func()) (inboxWatcherClaim, bool) {
	incumbent, live, err := room.Find(id.claimID)
	if err != nil || !live {
		return inboxWatcherClaim{}, false
	}
	if m := strings.ToLower(strings.TrimSpace(incumbent.Mode)); m != "foreman" && m != "peek" {
		return inboxWatcherClaim{}, false
	}
	// For a peek holder, also require the session digest to match — otherwise
	// a stale peek from a different CLAUDE_CODE_SESSION_ID would be adopted.
	if m := strings.ToLower(strings.TrimSpace(incumbent.Mode)); m == "peek" {
		if id.sessionClaim != "" && incumbent.SessionClaim != "" && incumbent.SessionClaim != id.sessionClaim {
			return inboxWatcherClaim{}, false
		}
		// Peek adoption is by session hash, not ancestry: the holder is a
		// short-lived child, not the harness parent, so ancestry would fail
		// even for the same CLAUDE_CODE_SESSION_ID. Hash match is sufficient.
		if id.sessionClaim != "" && incumbent.SessionClaim == id.sessionClaim {
			return inboxWatcherClaim{
				leave:     releaseLease,
				ownerLive: func() error { return nil },
			}, true
		}
	}
	anchor := inboxWatcherAnchor(incumbent)
	if inboxOwnerRelation(os.Getpid(), anchor) != inboxOwnerLive {
		return inboxWatcherClaim{}, false
	}
	return inboxWatcherClaim{
		leave: releaseLease,
		ownerLive: func() error {
			if inboxOwnerRelation(os.Getpid(), anchor) == inboxOwnerGone {
				return &inboxOwnerGoneError{agent: id.name, owner: anchor}
			}
			return nil
		},
	}, true
}

// localInboxWatchers holds the claim ids of this process's watchers while the
// coord ledger is bypassed (BASHY_CLAIM=0|off).
var localInboxWatchers sync.Map

// errInboxWatcherLive refuses a SECOND watcher for one identity, and the
// refusal has to say who the first one is.
//
// One registered name may have exactly one consuming reader, because two loops
// advance the same source cursors. That rule is right; the message was not. It
// named only the rule, so an external manager whose `sprint take --watch` was
// refused could not tell whether a real sibling watch was running, or whether a
// card from an earlier process in the SAME harness session was still on the
// board holding the name — the shape Sprint 412 and 329 both hit, where the
// holder is anchored to the harness parent PID and therefore looks live to
// every check. Without the holder's pid and mode there is nothing to act on.
//
// So the holder is looked up and reported. The lookup is best-effort: the
// refusal itself must never depend on the diagnostic succeeding, so a failed
// read just omits the detail.
func errInboxWatcherLive(id inboxWatcherIdentity) error {
	detail := ""
	if card, live, err := room.Find(id.claimID); err == nil && live {
		mode := strings.TrimSpace(card.Mode)
		if mode == "" {
			mode = "unknown"
		}
		owner := ""
		if anchor := inboxWatcherAnchor(card); anchor > 0 && anchor != card.PID {
			owner = fmt.Sprintf(", owned by the session at pid %d", anchor)
		}
		detail = fmt.Sprintf(" (holder: pid %d%s, mode %q, task %q)", card.PID, owner, mode, strings.TrimSpace(card.Task))
	}
	return fmt.Errorf("inbox: %s %q already has a live inbox watcher%s; one identity may have only one consuming reader, because two would advance the same cursors.\n"+
		"  see it:     `bashy agent list --all` (the watcher row for %[2]s) — nothing was consumed and no cursor moved\n"+
		"  stop it:    end that process; its card is retired when it exits\n"+
		"  one-shot:   `bashy inbox --as %[2]s --peek` reads without consuming and needs no watcher at all",
		id.kind, id.name, detail)
}

// retireInboxWatcherCard removes exactly this watcher's own card.
//
// room.Leave cannot: it presents the watcher's PID, but a card carrying a
// session digest is HELD by its OwnerPID (the agent session, which outlives
// every watcher it starts), so Leave sees a live card held by someone else and
// leaves it on the board — a restart in the same session is then refused until
// the session exits. The watcher's lifetime ends here; the session's does not.
// So retire as the card's actual holder, but only after confirming the card on
// disk is still the one this watcher wrote (same writer PID and session
// digest), so a stale exit never deletes a different holder's card.
func retireInboxWatcherCard(card room.Card) {
	cur, live, err := room.Find(card.ID)
	if err != nil || !live || cur.ID != card.ID || cur.PID != card.PID || cur.SessionClaim != card.SessionClaim {
		return
	}
	holder := cur.PID
	if cur.OwnerPID != 0 && cur.SessionClaim != "" {
		holder = cur.OwnerPID
	}
	room.LeavePID(cur.ID, holder)
}

// inboxWatcherIdentity is the resolved holder of one watcher lease. Two identity
// kinds may hold it: a registered AGENT, whose card carries a tool-session proof
// (SessionClaim) so one harness cannot claim another agent's name; and a
// registered PERSON, who carries none. A person is not an agent identity to
// impersonate — the guard SessionClaim defends is specifically AGENT-to-AGENT,
// and for a person the OS login is the trust boundary, exactly as
// bus.ResolveAuthoredActor already decides for authored mail. Everything else —
// the per-identity lease and the owning-session anchor for orphan detection — is
// generic and applies to both.
type inboxWatcherIdentity struct {
	kind         string // "registered agent" | "person", for the duplicate-watcher message
	name         string // canonical name; the lease holder and card nick
	claimID      string // room card id, namespaced so an agent and a person never share one
	principal    string // BASHY_PRINCIPAL (agent) or dhnt:person/<handle>
	sessionClaim string // empty for a person
	tool         string // empty for a person
	model        string // empty for a person
	binding      string // empty for a person
	nick         string
}

// inboxPersonClaimID namespaces a person's room card so it can never be mistaken
// for an agent's singleton claim (room.AgentClaimID) — the authored-actor guard
// looks agents up by that id, and a person must not answer to it.
func inboxPersonClaimID(handle string) string {
	return "person:" + room.AgentClaimID(handle)
}

// resolveInboxWatcherIdentity classifies a reader that resolveInboxReader has
// already vouched for. It accepts a registered agent or a registered person and
// refuses everything else: an arbitrary string, an observed-but-unregistered
// cursor, or a role must not file a watcher card. The refusal names BOTH
// registries so it never sends a human to a list they can never appear in.
func resolveInboxWatcherIdentity(reader string) (inboxWatcherIdentity, error) {
	if id, isInst := bus.ExplicitInstanceID(reader); isInst {
		inst, err := bus.InstanceStoreFn().Get(id)
		if err != nil {
			return inboxWatcherIdentity{}, fmt.Errorf("inbox: instance %s: %w", id, err)
		}
		if !inst.Active() {
			return inboxWatcherIdentity{}, fmt.Errorf("inbox: instance %s (%s) is retired", inst.UUID, inst.Label)
		}
		binding := strings.TrimSpace(inst.Family)
		if len(inst.Bindings) > 0 {
			binding = inst.Bindings[0]
		}
		toolName, modelName, _ := strings.Cut(binding, ":")
		if _, ok := fleet.New().Tool(toolName); !ok {
			toolName, modelName = bus.MainLineBase(inst.Label), ""
		}
		sess := bus.HashSessionClaim(currentAgentSession(inst.UUID))
		if sess == "" {
			sess = strings.TrimSpace(os.Getenv("BASHY_SESSION_CLAIM"))
		}
		nick := strings.TrimSpace(inst.Label)
		if nick == "" {
			nick = inst.UUID
		}
		return inboxWatcherIdentity{
			kind:         "instance",
			name:         inst.UUID,
			claimID:      inst.ClaimID(),
			principal:    inst.URN(),
			sessionClaim: sess,
			tool:         toolName,
			model:        modelName,
			binding:      binding,
			nick:         nick,
		}, nil
	}
	cat := fleet.New()
	if agent, ok := cat.Agent(reader); ok {
		return inboxWatcherIdentity{
			kind:         "registered agent",
			name:         agent.Name,
			claimID:      room.AgentClaimID(agent.Name),
			principal:    strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL")),
			sessionClaim: bus.HashSessionClaim(currentAgentSession(agent.Name)),
			tool:         agent.Tool,
			model:        agent.Model,
			binding:      agent.MatrixKey(),
			nick:         agent.Name,
		}, nil
	}
	if person, ok := cat.Person(reader); ok {
		return inboxWatcherIdentity{
			kind:      "person",
			name:      person.Handle,
			claimID:   inboxPersonClaimID(person.Handle),
			principal: principal.URN(principal.KindPerson, person.Handle, principal.LocalOwner),
			nick:      person.Handle,
		}, nil
	}
	return inboxWatcherIdentity{}, fmt.Errorf("inbox: watcher identity %q is not a registered Bashy agent or a known person; register an agent with `bashy agent add`, add a person with `bashy person add`, or choose one from `bashy agent list --all` / `bashy person list`", reader)
}

func acknowledgeInboxBatch(batch inboxBatch) error {
	for _, ack := range batch.acks {
		if err := ack(); err != nil {
			return fmt.Errorf("inbox: advance processed source cursor: %w", err)
		}
	}
	return nil
}

// snapshotUnifiedInbox only READS. Its ack closures are called after the whole
// rendered batch has reached stdout, so a broken pipe cannot silently consume a
// message. A snapshot containing only filtered outbound records has no output to
// fail and applies its closure silently. Each closure carries the exact
// per-source high-water mark observed.
// deliverSessionMail is the seam for the cross-host delivery pass; a test
// replaces it. nil means "no delivery on this host".
var deliverSessionMail = func(ctx context.Context, repoRoot, reader string) (weave.DeliveryReport, error) {
	return weave.DeliverRemoteMail(ctx, repoRoot, reader)
}

func snapshotUnifiedInbox(reader string, limit int, includeBus bool) (inboxBatch, error) {
	return snapshotInboxWithOptions(reader, limit, includeBus, true, false, true)
}

// snapshotRolesMu serializes the memoized role-table window below.
// bus.HostRoles is a process-global func var that the board scan consults on
// every directed post, so the swap must be exclusive. Snapshots never nest,
// so one mutex is enough.
var snapshotRolesMu sync.Mutex

// snapshotInbox is snapshotUnifiedInbox with the relay delivery pass optional:
// the per-command unread hint reads local stores only, so an ordinary command
// never waits on the network or files relay mail as a side effect.
func snapshotInbox(reader string, limit int, includeBus, deliver bool) (inboxBatch, error) {
	return snapshotInboxWithHint(reader, limit, includeBus, deliver, false)
}

func snapshotInboxWithHint(reader string, limit int, includeBus, deliver, hint bool) (inboxBatch, error) {
	return snapshotInboxWithOptions(reader, limit, includeBus, deliver, hint, false)
}

func snapshotInboxWithOptions(reader string, limit int, includeBus, deliver, hint, forTurn bool) (inboxBatch, error) {
	restoreRoles, conducted := memoizeHostRolesFor(reader)
	defer restoreRoles()
	var batch inboxBatch
	state, err := loadMailboxState(mailboxSpec{Key: "agent:" + reader, Address: reader, Kind: "agent"})
	if err != nil {
		return batch, err
	}

	// LOCAL DELIVERY FIRST (Sprint 217, the email model). Mail from another
	// host sits on the repo session's feed until this pass files it into the
	// reader's own board under its uuid; everything below then reads it as
	// ordinary directed mb mail. Delivery is not reading: it runs on --peek
	// too, never advances a read cursor, and a redelivery is a no-op. An
	// unpaired host, or one outside any checkout, skips it silently — the
	// inbox works exactly as before. A relay ERROR is reported as a warning,
	// never as an empty inbox: absence of evidence is not "no mail".
	if deliver && deliverSessionMail != nil {
		if cwd, err := os.Getwd(); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			rep, derr := deliverSessionMail(ctx, cwd, reader)
			cancel()
			switch {
			case derr != nil:
				batch.warns = append(batch.warns, fmt.Sprintf("session: not synced — %v (local mail unaffected)", derr))
			case rep.Skipped != "":
				// nothing to say: not a team checkout
			default:
				// "delivered 0" sat under a screenful of mail and read as a
				// contradiction: it counts records newly FILED this pass, not
				// records waiting. Say it only when something was filed.
				stamp := fmt.Sprintf("session %s: synced as of %s · cursor %s",
					shortSession(rep.Session), rep.AsOf.Format(time.RFC3339), shortSession(rep.Cursor))
				if rep.Delivered > 0 {
					stamp += fmt.Sprintf(" · delivered %d", rep.Delivered)
				}
				batch.warns = append(batch.warns, stamp)
			}
		}
	}
	var roleTopics map[string]string
	if limit > 0 || forTurn {
		roleTopics = hostRoleTopicsLocked()
		batch.roleTopics = roleTopics
	}
	appendLimited := func(source string, events []unifiedInboxEvent, ack func() error) {
		// Explicit ack is per record, not a source high-water mark: advancing
		// a cursor here would hide earlier unacknowledged mail. Filter before
		// limiting so acknowledged records cannot occupy the visible window.
		pending := events[:0]
		for _, event := range events {
			idSource := event.Source
			if event.Source == "meet" {
				idSource += ":" + event.Room
			}
			id := fmt.Sprintf("%s:%d", idSource, event.Seq)
			mark := state.Marks[id]
			if mark.AckedAt != "" || (forTurn && mark.DeliveredAt != "") {
				continue
			}
			// A seeded Meet copy shares the original MB acknowledgment.
			if event.Origin != nil && strings.EqualFold(event.Origin.Source, "mb") {
				origMark := state.Marks[fmt.Sprintf("mb:%d", event.Origin.Seq)]
				if origMark.AckedAt != "" || (forTurn && origMark.DeliveredAt != "") {
					continue
				}
			}
			pending = append(pending, event)
		}
		events = pending
		shown := events
		capped := limit > 0 && len(events) > limit
		if capped {
			shown = events[:limit]
			batch.warns = append(batch.warns, fmt.Sprintf("%s: %d more record(s) remain unread because of --limit", source, len(events)-limit))
		}
		batch.events = append(batch.events, shown...)
		if len(shown) == 0 || ack == nil {
			return
		}
		if !capped {
			batch.acks = append(batch.acks, ack)
			return
		}
		// A capped source has no safe whole-source watermark: it would consume
		// the omitted tail and any record that lands before the ack. Acknowledge
		// exactly the rendered records; the source cursor advances only over
		// their contiguous acknowledged prefix.
		for _, event := range shown {
			e := event
			batch.acks = append(batch.acks, func() error {
				return acknowledgeTurnInboxItem(reader, e, batch.rawEvents, roleTopics)
			})
		}
	}

	// Message board, including steward/conductor messages now addressed to the
	// stable role rather than copied into a role-specific store.
	var directed, other []bus.Post
	var incomplete bool
	if hint {
		directed, other, incomplete, err = bus.HintUnseen(reader)
		batch.hintIncomplete = incomplete
	} else {
		directed, other, _, err = bus.Unseen(reader, 0)
	}
	if err != nil {
		return batch, fmt.Errorf("message board: %w", err)
	}
	posts := append(directed, other...)
	mbEvents := make([]unifiedInboxEvent, 0, len(posts))
	var mbHigh, waitingHuntSeq int64
	for _, p := range posts {
		if p.IsHuntGroup() && bus.ClaimHolder(p.Seq) == "" && !bus.ReaderHasLiveSession(reader, p.To) {
			if waitingHuntSeq == 0 || p.Seq < waitingHuntSeq {
				waitingHuntSeq = p.Seq
			}
		}
		if p.Seq > mbHigh {
			mbHigh = p.Seq
		}
		mbEvents = append(mbEvents, unifiedInboxEvent{Schema: unifiedInboxSchema, Source: "mb", Seq: p.Seq, At: p.At, From: p.From, FromParty: p.FromParty, To: mbInboxTo(p), Topic: p.Topic, Body: p.Body, HuntGroup: p.IsHuntGroup()})
	}
	if waitingHuntSeq > 0 && mbHigh >= waitingHuntSeq {
		mbHigh = waitingHuntSeq - 1
	}
	var mbAck func() error
	if mbHigh > 0 {
		mbAck = func() error { return bus.MarkSeen(reader, mbHigh) }
	}
	appendLimited("mb", mbEvents, mbAck)

	// Every Meet room a reader is SEATED in is a channel. Deliverability keys on
	// the seat, never on the room's type: holding a seat IS a subscription to
	// it. Chaired rooms keep deciding who holds the FLOOR — they stop deciding
	// whether mail exists. The earlier `room.Board` gate was correct while a
	// chaired meeting's participants were spawned processes that died at the end
	// of their turn, so a transcript really was a record rather than an inbox;
	// long-lived seats retired that premise, and the exclusion then silenced the
	// one channel a sprint advertises to its own conductor. See
	// docs/agent-inbox-unified-delivery.md sections 1-2 and 7 (P0-a).
	rooms, err := inboxMeetRooms()
	if err != nil {
		return batch, fmt.Errorf("meet rooms: %w", err)
	}
	conductedRooms := conductedMeetRoomSet(conducted)
	for _, room := range rooms {
		if !stringMember(room.Members, reader) && !conductedRooms[strings.ToLower(strings.TrimSpace(room.ID))] {
			continue
		}
		seen := meet.SeenSeq(room.ID, reader)
		d, o, _, through, err := meet.UnreadRecords(room.ID, reader, 0)
		if err != nil {
			return batch, fmt.Errorf("meet room %s: %w", room.ID, err)
		}
		events := append(d, o...)
		out := make([]unifiedInboxEvent, 0, len(events))
		for _, record := range events {
			e := record.Event
			item := unifiedInboxEvent{Schema: unifiedInboxSchema, Source: "meet", Seq: record.Seq, At: e.TS.Format(time.RFC3339Nano), From: e.Speaker, To: e.To, Topic: e.Kind, Room: room.ID, Body: e.Text}
			if e.Origin != nil {
				item.Origin = &unifiedInboxOrigin{Source: e.Origin.Source, Seq: e.Origin.Seq}
			}
			out = append(out, item)
		}
		id := room.ID
		appendLimited("meet:"+id, out, func() error { return meet.MarkSeenThrough(id, reader, through) })
		if len(out) == 0 && through > seen {
			// UnreadRecords intentionally filters records authored by reader.
			// Carry their watermark as a silent acknowledgement: otherwise a
			// watch either busy-loops on its own post or has to render it merely
			// to move forward. runUnifiedInbox applies this ack without treating
			// it as an inbound batch.
			batch.acks = append(batch.acks, func() error { return meet.MarkSeenThrough(id, reader, through) })
		}
	}

	if includeBus {
		snapshot, err := bus.SnapshotInbox(reader)
		if err != nil {
			return batch, fmt.Errorf("bus notifications: %w", err)
		}
		appendLimited("bus", pendingEvents("bus", snapshot.Items), snapshot.Commit)
	}

	// Compatibility drain for pre-board role pings already durable in the old
	// role buffers. New role messages arrive through MB above; no sixth store is
	// created and this path can disappear after the retained backlog is empty.
	if bus.HostRoles != nil {
		for _, role := range bus.HostRoles() {
			if !(bus.Post{To: role.Topic}).Directed(reader) {
				continue
			}
			snapshot, err := bus.SnapshotInbox(role.Topic)
			if err != nil {
				return batch, fmt.Errorf("role %s: %w", role.Label, err)
			}
			out := pendingEvents("role:"+role.Label, snapshot.Items)
			appendLimited("role:"+role.Label, out, snapshot.Commit)
		}
	}
	for i := range batch.events {
		event := &batch.events[i]
		event.FromParty, event.FromHandle = inboxSenderIdentity(event.From, event.FromParty)
		event.EventType, event.StoryID, event.SuggestedVerb = classifyInboxEvent(event.Body, event.To)
	}
	batch.rawEvents = append([]unifiedInboxEvent(nil), batch.events...)
	batch.events = collapseProvenanceDuplicates(batch.events)
	sortInboxEvents(batch.events)
	return batch, nil
}

// parseInboxTime reads the timestamps the sources write (RFC3339, with or
// without fractional seconds). ok is false for an empty or foreign format.
func parseInboxTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// sortInboxEvents makes the stream chronological, newest last, by PARSED time.
// Sources format timestamps differently (fractional seconds, offsets), so a
// string compare mis-orders them. Records with no parseable time sort first,
// then ties break by source and sequence, so the order is deterministic.
func sortInboxEvents(events []unifiedInboxEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		ti, _ := parseInboxTime(events[i].At)
		tj, _ := parseInboxTime(events[j].At)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		if events[i].Source != events[j].Source {
			return events[i].Source < events[j].Source
		}
		return events[i].Seq < events[j].Seq
	})
}

// collapseProvenanceDuplicates removes only a Meet copy whose structured
// origin names an MB record present in the same rendered batch. Rendered prose
// is never a key: independently repeated text remains independently visible.
// Source acknowledgements stay in the batch, so successful rendering advances
// both the MB and Meet watermarks; a render failure advances neither.
func collapseProvenanceDuplicates(events []unifiedInboxEvent) []unifiedInboxEvent {
	mb := make(map[int64]struct{})
	for _, event := range events {
		if event.Source == "mb" {
			mb[event.Seq] = struct{}{}
		}
	}
	out := events[:0]
	for _, event := range events {
		if event.Source == "meet" && event.Origin != nil && strings.EqualFold(event.Origin.Source, "mb") {
			if _, ok := mb[event.Origin.Seq]; ok {
				continue
			}
		}
		out = append(out, event)
	}
	return out
}

func pendingEvents(source string, items []bus.Pending) []unifiedInboxEvent {
	out := make([]unifiedInboxEvent, 0, len(items))
	for _, p := range items {
		out = append(out, unifiedInboxEvent{Schema: unifiedInboxSchema, Source: source, Seq: p.Seq, At: p.TS, From: p.Principal, To: p.To, Topic: p.Topic, Room: p.Room, Body: p.Body})
	}
	return out
}

// Preserve send-time provenance. Older UUID-only records can recover their
// frozen family from the instance store, but a reusable label is never looked
// up as an identity. Handles are display metadata resolved only by UUID.
func inboxSenderIdentity(from string, party *bus.Party) (*bus.Party, string) {
	id, ok := bus.ExplicitInstanceID(from)
	if party != nil {
		id, ok = party.UUID, true
	}
	if !ok {
		return party, ""
	}
	inst, err := bus.InstanceStoreFn().Get(id)
	if err != nil {
		return party, ""
	}
	if party == nil {
		party = &bus.Party{UUID: inst.UUID, Label: inst.Label, FamilyID: inst.FamilyID,
			Family: inst.Family, Policy: inst.Policy, Bindings: inst.Bindings}
	}
	return party, inst.Handle
}

// Both inbox views filter exactly the attribution that they display.
func inboxSenderText(from string, party *bus.Party, handle string) string {
	text := emptyAs(from, "unknown")
	if party == nil {
		return text
	}
	var details []string
	if handle != "" {
		details = append(details, "handle: "+handle)
	} else if party.Label != "" && party.Label != from {
		details = append(details, "label: "+party.Label)
	}
	details = append(details, "uuid: "+party.UUID)
	if party.Family != "" {
		details = append(details, "family: "+party.Family)
	}
	if len(party.Bindings) > 0 {
		details = append(details, "bindings: "+strings.Join(party.Bindings, ", "))
	}
	if party.Selected != "" {
		details = append(details, "selected: "+party.Selected)
	}
	return text + " (" + strings.Join(details, "; ") + ")"
}

func renderInboxBatch(out, errOut io.Writer, batch inboxBatch, jsonOut bool) error {
	var rendered bytes.Buffer
	if jsonOut {
		enc := json.NewEncoder(&rendered)
		for _, event := range batch.events {
			if err := enc.Encode(event); err != nil {
				return err
			}
		}
	} else {
		for _, event := range batch.events {
			where := event.Source
			if event.Room != "" {
				where += "/" + event.Room
			}
			fmt.Fprintf(&rendered, "%s [%s:%d] %s → %s", emptyAs(event.At, "-"), where, event.Seq, inboxSenderText(event.From, event.FromParty, event.FromHandle), emptyAs(event.To, "all"))
			if event.Topic != "" {
				fmt.Fprintf(&rendered, " (%s)", event.Topic)
			}
			fmt.Fprintf(&rendered, "\n%s\n\n", event.Body)
		}
	}
	n, err := out.Write(rendered.Bytes())
	if err != nil {
		return err
	}
	if n != rendered.Len() {
		return io.ErrShortWrite
	}
	for _, warning := range batch.warns {
		fmt.Fprintln(errOut, warning)
	}
	return nil
}

// shortSession abbreviates a uuid or cursor for the one-line sync stamp.
func shortSession(id string) string {
	if len(id) > 13 {
		return id[:13]
	}
	if id == "" {
		return "-"
	}
	return id
}

func stringMember(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func claimAdmittedHuntEvents(reader string, events []unifiedInboxEvent) []unifiedInboxEvent {
	claimed := make([]unifiedInboxEvent, 0, len(events))
	for _, e := range events {
		if !claimHuntInboxEvent(reader, e) {
			continue
		}
		claimed = append(claimed, e)
	}
	return claimed
}

// unifiedTurnPreamble is wired into coreutils/chat. Bus pending is already
// drained by bus.TurnPreamble itself; this callback adds every other source in
// one bounded block and acknowledges only after rendering that block in memory.
func unifiedTurnPreamble(agent string) bus.PreparedPreamble {
	batch, err := snapshotUnifiedInbox(agent, 0, false)
	if err != nil {
		warning := fmt.Sprintf("[Bashy unified inbox warning]\nCould not read every inbound source: %v\nNo source cursor was advanced; run `bashy inbox --as %s`.", err, agent)
		return bus.NewPreparedPreamble(warning, nil)
	}
	batch.events = claimAdmittedHuntEvents(agent, batch.events)
	if len(batch.events) == 0 {
		return bus.PreparedPreamble{}
	}
	var out bytes.Buffer
	if err := renderInboxBatch(&out, io.Discard, batch, false); err != nil {
		return bus.PreparedPreamble{}
	}
	ack := func() error {
		for _, ack := range batch.acks {
			if err := ack(); err != nil {
				return err
			}
		}
		return nil
	}
	text := "[Bashy unified inbox — read before the instruction below]\n" + strings.TrimSpace(out.String())
	return bus.NewPreparedPreamble(text, ack)
}
