package agentos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/lockfile"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/room"
	"github.com/spf13/cobra"
)

const unifiedInboxSchema = "bashy-inbox-v1"

const inboxWatcherMode = "inbox"

var inboxMeetRooms = meet.Rooms

type unifiedInboxEvent struct {
	Schema string              `json:"schema"`
	Source string              `json:"source"`
	Seq    int64               `json:"seq"`
	At     string              `json:"at,omitempty"`
	From   string              `json:"from,omitempty"`
	To     string              `json:"to,omitempty"`
	Topic  string              `json:"topic,omitempty"`
	Room   string              `json:"room,omitempty"`
	Body   string              `json:"body"`
	Origin *unifiedInboxOrigin `json:"origin,omitempty"`
}

type unifiedInboxOrigin struct {
	Source string `json:"source"`
	Seq    int64  `json:"seq"`
}

type inboxBatch struct {
	events []unifiedInboxEvent
	acks   []func() error
	warns  []string
}

func newUnifiedInboxCmd() *cobra.Command {
	var as string
	var wait time.Duration
	var peek, jsonOut, watch bool
	var limit int
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "read or watch every inbound Bashy communication channel",
		Long: `inbox is one read-through view over the existing message board, Meet
boards, Bus pending buffers, and steward/conductor role addresses. It creates no
message store and keeps each source's own cursor.

  bashy inbox                         drain everything waiting for you
  bashy inbox --peek                  inspect without advancing any cursor
  bashy inbox --wait 15m              wait for one batch, then return
  bashy inbox --watch                 follow new inputs until interrupted
  bashy inbox --watch --wait 15m      follow for at most 15 minutes

For a durable, searchable mailbox that never disappears into turn or console
traffic, use the explicit mailbox operations. Listing and searching consume
nothing; read keeps a record pending; only ack removes it from the pending view.

  bashy inbox list --topic harness --search profile
  bashy inbox read mb:42
  bashy inbox ack mb:42
  bashy inbox human list --topic posix-cert --project dhnt
  bashy inbox human send --topic posix-cert --project dhnt --status blocked --ref docs/status.md "Profile D needs review"

The human lane belongs to the current OS user and aggregates MB, Meet boards,
Bus/ping notifications, and broadcasts with its own state. Agent reads cannot
consume it; authorized local agents may query and organize that same state for
the human. Keep status concise and put detail at a stable shared reference.

For a Bashy-managed chat session, unified input is automatically injected through
the session's real control transport and acknowledged only after delivery. An
external sprint manager instead uses 'bashy sprint take/start --watch': the sprint
command stays attached to its agent-harness parent and streams the same events.
For other external orchestration that can retain and actively poll a process,
run 'bashy inbox --as NAME --watch --json' and poll its output at every turn.
Never detach and ignore it: rendered records advance NAME's cursors. While it
runs, the watcher appears as active in 'bashy agents'; second watcher cannot claim
the same NAME. If the external harness cannot retain and poll a process,
repeat 'bashy inbox --as NAME --watch --wait 60s --json', process its streamed
batches, and immediately re-enter. --watch makes every bounded run hold NAME's
claim; one empty timeout does not end active monitoring.

Assign a model-driven sentinel one distinct registered Bashy identity (verify
with 'bashy agents show NAME'), invite it to assigned Meet boards, and
route/subscribe its own inputs. Surface every request promptly;
prioritize directed, BLOCKED, CONFLICT, ownership, baseline, and merge inputs.
If action is not immediate, acknowledge receipt with owner, action, and ETA.
Never read as another identity, silently consume, duplicate claimed work, or
impersonate decision authority. A sentinel sees only sources routed to its own
identity; invite/subscribe/address it explicitly. Its reply must say the
sentinel routed the request and the supervisor has not read it. On expiry,
handoff processed/outstanding counts and last source sequences. See
'bashy skills show check-messages'.

NAME owns the address and cursor; NICK/aliases do not create another inbox.
Never share one registered NAME between agents. Separate concurrent topic
watchers need separate registered names. Keep messages short: request/decision,
priority, owner/expected response, and a stable repo-relative + commit/issue/
room/artifact reference; never send only an inaccessible temporary path.

The live watcher card is also NAME's cooperative authored-message claim. MB,
Meet, ping, notify, Bus publish, and human-mailbox send refuse an explicit --as
NAME from a different live agent session and notify NAME of the refused attempt.
A governed tool session uses a hashed session claim; tools without stable session
metadata fall back to the watcher parent's process lineage. BASHY_PRINCIPAL is
attribution, not ownership proof. This is host-local collision prevention, not
cryptographic identity.
Inspect ownership with 'bashy whois agent:NAME' (TAKEN) and 'bashy agents'.

MB post/send (including messaging ping), Bus publish, and every manual Meet tell
accept at most 1024 UTF-8
bytes per authored body and never truncate or auto-split. If no stable shared
reference exists, manually number <=1024-byte parts with one correlation token;
the receiver waits for END and reports missing parts.

A watch registered to NAME stops itself once the agent session that started it
is no longer its parent or ancestor. It stops BEFORE reading, so nothing is
rendered or acknowledged, and releases NAME's card and claim so a live session
can resume coverage. A recycled owner pid is not that session; where the process
tree cannot be read the watch keeps running.

A sentinel that exits must say monitoring ENDED, why/deadline, last processed
provenance, outstanding status, and who resumes coverage. It must never promise
continued monitoring after its process or assignment ends.

Bashy-owned agent sessions receive the same view once at each real turn
boundary. A session started outside Bashy has no authenticated control channel
to steer; use this command (or --watch) explicitly. Bashy never guesses a PID or
pretends such a session was adopted.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if wait < 0 {
				return fmt.Errorf("inbox: --wait must not be negative")
			}
			if limit < 0 {
				return fmt.Errorf("inbox: --limit must not be negative")
			}
			if watch && peek {
				return fmt.Errorf("inbox: --watch cannot be combined with --peek (the same unread batch would repeat forever)")
			}
			if watch && limit > 0 {
				return fmt.Errorf("inbox: --watch cannot be combined with --limit (a capped source intentionally remains unread)")
			}
			reader, err := resolveInboxReader(as)
			if err != nil {
				return err
			}
			// A bare human watch belongs to the OS login and has no agent card.
			// An explicit --as or authenticated agent watch must instead claim
			// one registered, globally unique fleet identity.
			principal := strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
			var claim inboxWatcherClaim
			if watch && (strings.TrimSpace(as) != "" || strings.Contains(principal, "agent/")) {
				registered, err := registerInboxWatcher(reader)
				if err != nil {
					return err
				}
				// The release runs on EVERY exit, the orphan exit included: a
				// watcher that stops because its session died must hand the
				// identity back, or the replacement it just told the fleet to
				// start is refused by the corpse's own claim.
				defer registered.leave()
				claim = registered
			}
			// After the claim, never before it: the follow runtime arms native
			// filesystem watches, and a refused claim must not leave them (and
			// their goroutine) behind on a path that never reaches the loop
			// that closes them.
			poll := defaultInboxPollRuntime(watch || wait > 0)
			poll.ownerLive = claim.ownerLive
			return runUnifiedInboxWithPoll(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), reader, limit, peek, jsonOut, watch, wait, poll)
		},
	}
	f := cmd.Flags()
	f.StringVar(&as, "as", "", "read as this identity (required when an external agent session cannot be attributed)")
	f.DurationVar(&wait, "wait", 0, "wait up to this duration for input (with --watch: total watch bound)")
	f.BoolVar(&peek, "peek", false, "read without advancing any source cursor")
	f.BoolVar(&watch, "watch", false, "follow all inbound sources until interrupted")
	f.IntVarP(&limit, "limit", "n", 0, "show at most this many records per source (0 = no cap; a capped source remains unread)")
	f.BoolVar(&jsonOut, "json", false, "emit one "+unifiedInboxSchema+" object per line (NDJSON)")
	cmd.AddCommand(newMailboxListCmd(false), newMailboxReadCmd(false), newMailboxAckCmd(false), newMailboxPreserveCmd(false), newMailboxOrganizeCmd(false), newHumanMailboxCmd())
	cmd.CompletionOptions.DisableDefaultCmd = true
	return cmd
}

// resolveInboxReader prevents an authenticated Bashy agent from borrowing a
// different identity's cursors. External sessions may name themselves, but the
// name must resolve to a registered agent rather than minting an arbitrary
// cursor by typo. Role backlog is reachable only through its current holder.
func resolveInboxReader(as string) (string, error) {
	principal := strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
	if strings.Contains(principal, "agent/") {
		self, err := bus.BoardIdentity("")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(as) == "" {
			return self, nil
		}
		requested, err := bus.BoardIdentity(as)
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(self, requested) {
			return "", fmt.Errorf("inbox: authenticated agent %q cannot read as %q; each registered name owns its own cursors", self, requested)
		}
		return self, nil
	}
	if strings.TrimSpace(as) == "" {
		return bus.BoardIdentity("")
	}
	addr, kind, ok := bus.ResolveSendTarget(as)
	if !ok || kind != bus.TargetAgent {
		return "", fmt.Errorf("inbox: --as %q is not a registered Bashy agent; verify with `bashy agents list --all` and `bashy whois agent:%s`", as, as)
	}
	return addr, nil
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

// registerInboxWatcher makes a persistent inbox reader visible through
// `bashy agents` for exactly as long as its watch process is alive. The stable
// card ID is also a claim: two processes may not consume one registered
// identity's cursors concurrently.
func registerInboxWatcher(reader string) (inboxWatcherClaim, error) {
	return registerInboxWatcherAs(reader, inboxWatcherMode, "watching Bashy inbox", nil)
}

func registerSprintInboxWatcher(reader string) (inboxWatcherClaim, error) {
	return registerInboxWatcherAs(reader, "sprint-inbox", "managing sprint with attached inbox stream",
		[]string{room.CapInboxStream})
}

func registerInboxWatcherAs(reader, mode, task string, caps []string) (inboxWatcherClaim, error) {
	agent, ok := fleet.New().Agent(reader)
	if !ok {
		return inboxWatcherClaim{}, fmt.Errorf("inbox: watcher identity %q is not a registered Bashy agent; register it with `bashy agents add` or choose one from `bashy agents list --all`", reader)
	}

	// The kernel lock is the watcher lease. It closes the read-before-write
	// race between two fresh processes and also refuses two watcher loops in
	// one process, since both would otherwise advance the same source cursors.
	claimDir := filepath.Join(room.Dir(), "claims")
	if err := os.MkdirAll(claimDir, 0o700); err != nil {
		return inboxWatcherClaim{}, fmt.Errorf("inbox: prepare watcher claims: %w", err)
	}
	claimPath := filepath.Join(claimDir, fmt.Sprintf("inbox-%x.lock", sha256.Sum256([]byte(agent.Name))))
	claim, err := lockfile.TryAcquire(claimPath, lockfile.Holder{Name: agent.Name, Intent: "watch inbox"})
	if err != nil {
		if errors.Is(err, lockfile.ErrHeld) {
			return inboxWatcherClaim{}, fmt.Errorf("inbox: registered agent %q already has a live inbox watcher", agent.Name)
		}
		return inboxWatcherClaim{}, fmt.Errorf("inbox: claim watcher identity %q: %w", agent.Name, err)
	}
	cwd, _ := os.Getwd()
	principal := strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
	ownerPID := os.Getppid()
	if ownerPID <= 1 {
		// PID 1 is a common ancestor, never a session proof. Leave OwnerPID empty
		// so the shared guard falls back to this watcher's live PID and fails
		// closed for sibling commands when no stronger tool-session claim exists.
		ownerPID = 0
	}
	sessionClaim := bus.HashSessionClaim(currentAgentSession(agent.Name))
	card := room.Card{
		// The registered name is the global identity claim. A parallel
		// "inbox:NAME" card would let one identity occupy two live sessions.
		ID:           room.AgentClaimID(agent.Name),
		Principal:    principal,
		SessionClaim: sessionClaim,
		Tool:         agent.Tool,
		Model:        agent.Model,
		Binding:      agent.MatrixKey(),
		Nick:         agent.Name,
		Mode:         mode,
		Task:         task,
		Caps:         caps,
		PID:          os.Getpid(),
		OwnerPID:     ownerPID,
		Cwd:          cwd,
	}
	if err := room.Join(card); err != nil {
		_ = claim.Release()
		return inboxWatcherClaim{}, fmt.Errorf("inbox: register watcher %q: %w", agent.Name, err)
	}
	anchor := inboxWatcherAnchor(card)
	return inboxWatcherClaim{
		leave: func() {
			// Both halves, always. The room card is what `bashy agents` and the
			// authored-identity guard read; the kernel claim is what the next
			// watcher process must take. Releasing one and keeping the other
			// leaves the identity half-held, which reads as available in one
			// surface and taken in the other.
			room.Leave(card.ID)
			_ = claim.Release()
		},
		ownerLive: func() error {
			if inboxOwnerRelation(os.Getpid(), anchor) == inboxOwnerGone {
				return &inboxOwnerGoneError{agent: agent.Name, owner: anchor}
			}
			return nil
		},
	}, nil
}

func runUnifiedInbox(ctx context.Context, out, errOut io.Writer, reader string, limit int, peek, jsonOut, watch bool, bound time.Duration) error {
	return runUnifiedInboxWithPoll(ctx, out, errOut, reader, limit, peek, jsonOut, watch, bound, defaultInboxPollRuntime(watch || bound > 0))
}

func runUnifiedInboxWithPoll(ctx context.Context, out, errOut io.Writer, reader string, limit int, peek, jsonOut, watch bool, bound time.Duration, poll inboxPollRuntime) error {
	if poll.close != nil {
		defer poll.close()
	}
	deadline := time.Time{}
	if bound > 0 {
		deadline = poll.now().Add(bound)
	}
	// Reading every source is expensive enough that doing it on a fixed short
	// timer saturates a core on an idle host; the gate follows native store
	// notifications and periodically rescans as a correctness backstop. See
	// inbox_poll.go.
	gate := &inboxPollGate{reader: reader, fingerprint: poll.fingerprint, fullRescan: poll.fullRescan}
	interval := poll.min
	for {
		// BEFORE the read, not after it. Rendering is what advances a cursor,
		// so a watcher whose owning session has exited must discover that
		// while the backlog is still intact — a record drained into a dead
		// session's stdout is gone from every other reader's view too.
		if poll.ownerLive != nil {
			if err := poll.ownerLive(); err != nil {
				return err
			}
		}
		now := poll.now()
		read, changed, sum, sampled := gate.due(now)
		if changed {
			// Traffic just landed: stay responsive for whatever follows it.
			interval = poll.min
		}
		if read {
			batch, err := poll.snapshot(reader, limit, true)
			if err != nil {
				return err
			}
			gate.commit(sum, sampled, now)
			if len(batch.events) > 0 {
				if err := renderInboxBatch(out, errOut, batch, jsonOut); err != nil {
					return err
				}
				if !peek {
					if err := acknowledgeInboxBatch(batch); err != nil {
						return err
					}
				}
				interval = poll.min
				if !watch {
					return nil
				}
			} else {
				// Some source records are deliberately not inbound — most notably
				// this reader's own Meet posts. Their source cursor still has to
				// pass them or every poll rediscovers the same outbound record, but
				// they must not render, wake a wait, or end a bounded read.
				if !peek && len(batch.acks) > 0 {
					if err := acknowledgeInboxBatch(batch); err != nil {
						return err
					}
				}
				if !watch && bound == 0 {
					fmt.Fprintf(errOut, "nothing new in any channel for %s\n", reader)
					return nil
				}
			}
		}
		now = poll.now()
		if !watch && !deadline.IsZero() && !now.Before(deadline) {
			fmt.Fprintln(errOut, "EMPTY (timeout)")
			return nil
		}
		if watch && !deadline.IsZero() && !now.Before(deadline) {
			return nil
		}
		pause := interval
		if !deadline.IsZero() && now.Add(pause).After(deadline) {
			pause = deadline.Sub(now)
		}
		if err := poll.wait(ctx, pause); err != nil {
			if watch {
				return nil
			}
			return err
		}
		// Back off while nothing is arriving. Delivery latency stays bounded
		// by inboxPollMax; an idle timeout only reads an atomic generation.
		interval *= 2
		if interval > poll.max {
			interval = poll.max
		}
	}
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
func snapshotUnifiedInbox(reader string, limit int, includeBus bool) (inboxBatch, error) {
	var batch inboxBatch
	appendLimited := func(source string, events []unifiedInboxEvent, ack func() error) {
		shown := events
		capped := limit > 0 && len(events) > limit
		if capped {
			shown = events[:limit]
			batch.warns = append(batch.warns, fmt.Sprintf("%s: %d more record(s) remain unread because of --limit", source, len(events)-limit))
		}
		batch.events = append(batch.events, shown...)
		if len(shown) > 0 && !capped && ack != nil {
			batch.acks = append(batch.acks, ack)
		}
	}

	// Message board, including steward/conductor messages now addressed to the
	// stable role rather than copied into a role-specific store.
	directed, other, _, err := bus.Unseen(reader, 0)
	if err != nil {
		return batch, fmt.Errorf("message board: %w", err)
	}
	posts := append(directed, other...)
	mbEvents := make([]unifiedInboxEvent, 0, len(posts))
	var mbHigh int64
	for _, p := range posts {
		if p.Seq > mbHigh {
			mbHigh = p.Seq
		}
		mbEvents = append(mbEvents, unifiedInboxEvent{Schema: unifiedInboxSchema, Source: "mb", Seq: p.Seq, At: p.At, From: p.From, To: bus.RoleLabelFor(p.To), Topic: p.Topic, Body: p.Body})
	}
	appendLimited("mb", mbEvents, func() error { return bus.MarkSeen(reader, mbHigh) })

	// Every Meet board is a channel. Ordinary chaired meetings are deliberately
	// excluded: their transcript is a record, not a participant inbox.
	rooms, err := inboxMeetRooms()
	if err != nil {
		return batch, fmt.Errorf("meet rooms: %w", err)
	}
	for _, room := range rooms {
		if !room.Board || !stringMember(room.Members, reader) {
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
			if !strings.EqualFold(strings.TrimSpace(role.Holder), strings.TrimSpace(reader)) {
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
	batch.events = collapseProvenanceDuplicates(batch.events)
	return batch, nil
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
			fmt.Fprintf(&rendered, "[%s:%d] %s → %s", where, event.Seq, emptyAs(event.From, "unknown"), emptyAs(event.To, "all"))
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

// unifiedTurnPreamble is wired into coreutils/chat. Bus pending is already
// drained by bus.TurnPreamble itself; this callback adds every other source in
// one bounded block and acknowledges only after rendering that block in memory.
func unifiedTurnPreamble(agent string) bus.PreparedPreamble {
	batch, err := snapshotUnifiedInbox(agent, 0, false)
	if err != nil {
		warning := fmt.Sprintf("[Bashy unified inbox warning]\nCould not read every inbound source: %v\nNo source cursor was advanced; run `bashy inbox --as %s`.", err, agent)
		return bus.NewPreparedPreamble(warning, nil)
	}
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
