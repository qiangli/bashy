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
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/meet"
	"github.com/qiangli/yoke/pkg/principal"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

const unifiedInboxSchema = "bashy-inbox-v1"

const inboxWatcherMode = "inbox"

var inboxMeetRooms = meet.Rooms

type unifiedInboxEvent struct {
	Schema     string              `json:"schema"`
	Source     string              `json:"source"`
	Seq        int64               `json:"seq"`
	At         string              `json:"at,omitempty"`
	From       string              `json:"from,omitempty"`
	FromParty  *bus.Party          `json:"from_party,omitempty"`
	FromHandle string              `json:"from_handle,omitempty"`
	To         string              `json:"to,omitempty"`
	Topic      string              `json:"topic,omitempty"`
	Room       string              `json:"room,omitempty"`
	Body       string              `json:"body"`
	Origin     *unifiedInboxOrigin `json:"origin,omitempty"`
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
	var filter mailboxFilter
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "read or watch every inbound Bashy communication channel",
		Long: `inbox is one read-through view over the existing message board, Meet
boards, Bus pending buffers, and steward/conductor role addresses. It creates no
message store and keeps each source's own cursor.

Reading never consumes. Printing a record - to a terminal, a pipe, or --json -
leaves every source cursor and mailbox read/ack state exactly as it was. One
chronological stream, newest last; each record shows its time, source and sender.

  bashy inbox                         what is waiting for you, oldest first
  bashy inbox --from alice            only messages whose sender matches
  bashy inbox --search profile        only messages containing this text
  bashy inbox --since 2h              only messages from the last two hours
  bashy inbox --wait 15m              wait for one batch (still not consumed)
  bashy inbox --watch                 follow new inputs; delivery acknowledges them
  bashy inbox --watch --wait 15m      follow for at most 15 minutes

Only an explicit acknowledgment changes state: 'bashy inbox ack ID' for a
mailbox record, or --watch (and a Bashy-managed session's turn delivery), which
advances a source cursor after the record was actually delivered. --from,
--search and --since select what is shown, so they cannot be combined with
--watch (it would acknowledge records you never saw). --peek is accepted and
is the default.

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
runs, the watcher appears as active in 'bashy agent'; second watcher cannot claim
the same NAME. If the external harness cannot retain and poll a process,
repeat 'bashy inbox --as NAME --watch --wait 60s --json', process its streamed
batches, and immediately re-enter. --watch makes every bounded run hold NAME's
claim; one empty timeout does not end active monitoring.

Assign a model-driven sentinel one distinct registered Bashy identity (verify
with 'bashy agent show NAME'), invite it to assigned Meet boards, and
route/subscribe its own inputs. Surface every request promptly;
prioritize directed, BLOCKED, CONFLICT, ownership, baseline, and merge inputs.
If action is not immediate, acknowledge receipt with owner, action, and ETA.
Never read as another identity, silently consume, duplicate claimed work, or
impersonate decision authority. A sentinel sees only sources routed to its own
identity; invite/subscribe/address it explicitly. Its reply must say the
sentinel routed the request and the supervisor has not read it. On expiry,
handoff processed/outstanding counts and last source sequences. See
'bashy skill show inbox'.

NAME owns the address and cursor; NICK/aliases do not create another inbox.
Never share one registered NAME between agents. Separate concurrent topic
watchers need separate registered names. Keep messages short: request/decision,
priority, owner/expected response, and a stable repo-relative + commit/issue/
room/artifact reference; never send only an inaccessible temporary path.

The live watcher card is also NAME's cooperative authored-message claim. MB,
Meet, ping, notify, Bus publish, and human-mailbox send refuse an explicit --as
NAME from a different live agent session and notify NAME of the refused attempt.
A governed tool session uses a hashed session claim; tools without stable session
metadata fall back to the watcher parent's process lineage. A seat nobody holds
is taken by the first authored message from a process that declares the name
(BASHY_AGENT=NAME in its environment — the same declaration that seats NAME@host
on a repo session); an undeclared caller is refused and told so. BASHY_PRINCIPAL
is attribution, not ownership proof. This is host-local collision prevention, not
cryptographic identity.
Inspect ownership with 'bashy whois agent:NAME' (TAKEN) and 'bashy agent'.

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
			if err := filter.validate(time.Now()); err != nil {
				return err
			}
			if watch && filter.active() {
				return fmt.Errorf("inbox: --watch cannot be combined with --from/--search/--since (delivery acknowledges records the filter would hide)")
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
			if filter.active() {
				poll.snapshot = filteredInboxSnapshot(poll.snapshot, filter, limit)
			}
			// Only a watch consumes: it keeps following, so a peek-only batch
			// would repeat forever. Every other read just prints.
			return runUnifiedInboxWithPoll(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), reader, limit, peek || !watch, jsonOut, watch, wait, poll)
		},
	}
	f := cmd.Flags()
	f.StringVar(&as, "as", "", "read as this identity (required when an external agent session cannot be attributed)")
	f.DurationVar(&wait, "wait", 0, "wait up to this duration for input (with --watch: total watch bound)")
	f.BoolVar(&peek, "peek", false, "read without advancing any source cursor (the default; only --watch consumes)")
	f.StringVar(&filter.from, "from", "", "only messages whose sender contains this text")
	f.StringVar(&filter.search, "search", "", "only messages containing this text (body/sender/recipient/topic/room/source)")
	f.StringVar(&filter.sinceRaw, "since", "", "only messages at or after this time (duration like 2h or 3d, or RFC3339 / YYYY-MM-DD)")
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
	if id, ok := principal.SelfInstanceUUID(); ok {
		canonical, err := fleet.ParseInstanceUUID(id)
		if err != nil {
			return "", fmt.Errorf("inbox: invalid session instance: %w", err)
		}
		self := fleet.InstanceAddressPrefix + canonical
		if strings.TrimSpace(as) != "" {
			requested, explicit := bus.ExplicitInstanceID(as)
			if !explicit || requested != canonical {
				return "", fmt.Errorf("inbox: instance %q cannot read as %q; each instance owns its own mailbox. %s", self, as, inboxReaderHint)
			}
		}
		return self, nil
	}
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
			return "", fmt.Errorf("inbox: authenticated agent %q cannot read as %q; each registered name owns its own cursors. %s", self, requested, inboxReaderHint)
		}
		return self, nil
	}
	if strings.TrimSpace(as) == "" {
		return bus.BoardIdentity("")
	}
	// A PERSON owns a mailbox too. Restricting the reader to an agent left the
	// operator with no inbox at all: mail addressed to them was written and
	// nothing could read it, while `bashy app` showed the same rows to everyone
	// as a third-person peek. `bus.TargetPerson` already existed and was simply
	// refused here.
	//
	// The rule is by EXCLUSION, and only a role is excluded: it is an address
	// that survives handover and has no cursor of its own to advance.
	//
	// Listing the allowed kinds instead was subtly wrong. A name that ALREADY
	// holds a cursor resolves as bus.TargetReader — before the agent or person
	// branches are reached — so the operator's own name, which had a cursor from
	// every prior read, was refused while an alias of the same person passed.
	// Someone who owns a cursor is by definition an inbox owner.
	addr, kind, ok := bus.ResolveSendTarget(as)
	if !ok || kind == bus.TargetRole {
		return "", fmt.Errorf("inbox: --as %q owns no mailbox here; establish an identity first — declare BASHY_AGENT=NAME (or register with `bashy agent add` / `bashy person add`), then choose an agent from `bashy agent list --all` or a person from `bashy person list` (`bashy whois %s` says what it resolves to). %s", as, as, inboxReaderHint)
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
// `bashy agent` for exactly as long as its watch process is alive. The stable
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
	id, err := resolveInboxWatcherIdentity(reader)
	if err != nil {
		return inboxWatcherClaim{}, err
	}

	// The kernel lock is the watcher lease. It closes the read-before-write
	// race between two fresh processes and also refuses two watcher loops in
	// one process, since both would otherwise advance the same source cursors.
	// This guard is generic — a person must not run two concurrent watchers of
	// their own cursors any more than an agent may — so it is keyed on the
	// resolved claim id, which is namespaced per identity kind.
	claimDir := filepath.Join(room.Dir(), "claims")
	if err := os.MkdirAll(claimDir, 0o700); err != nil {
		return inboxWatcherClaim{}, fmt.Errorf("inbox: prepare watcher claims: %w", err)
	}
	claimPath := filepath.Join(claimDir, fmt.Sprintf("inbox-%x.lock", sha256.Sum256([]byte(id.claimID))))
	claim, err := lockfile.TryAcquire(claimPath, lockfile.Holder{Name: id.name, Intent: "watch inbox"})
	if err != nil {
		if errors.Is(err, lockfile.ErrHeld) {
			return inboxWatcherClaim{}, fmt.Errorf("inbox: %s %q already has a live inbox watcher", id.kind, id.name)
		}
		return inboxWatcherClaim{}, fmt.Errorf("inbox: claim watcher identity %q: %w", id.name, err)
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
		_ = claim.Release()
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
			room.Leave(card.ID)
			_ = claim.Release()
		},
		ownerLive: func() error {
			if inboxOwnerRelation(os.Getpid(), anchor) == inboxOwnerGone {
				return &inboxOwnerGoneError{agent: id.name, owner: anchor}
			}
			return nil
		},
	}, nil
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

// refreshSprintOwnerActivity is the one refresher, behind a var so a test can
// observe the watch's heartbeat without a sprint store on disk. Same shape as
// meet's apiRunner and operableFn: one package-level var, overridden and
// restored by the test that needs it.
var refreshSprintOwnerActivity = weave.RefreshSprintOwnerActivity

func runUnifiedInbox(ctx context.Context, out, errOut io.Writer, reader string, limit int, peek, jsonOut, watch bool, bound time.Duration) error {
	// READING YOUR MAIL IS THE HEARTBEAT. An agent that reads its inbox is
	// demonstrably running and demonstrably attending to this channel, which is
	// what a seat needs to be true — and it is something an agent already does
	// at a turn boundary rather than a process it has to hold open. Best-effort
	// and silent: bookkeeping the caller did not ask for may never fail a read.
	refreshSprintOwnerActivity(reader)
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
	// THE WATCH MUST KEEP REFRESHING, not refresh once and drift.
	//
	// The refresh above runs before this loop, which was enough for a bounded
	// read and wrong for a watch: measured on this sprint's own seat, a watch
	// running 1h18m showed a lease STALE for 43m — the time since the last
	// bounded command, not since the watch started. So an agent following the
	// instruction the tool itself prints ("`--watch` to stay attached") still
	// went stale, and that instruction is what REPLACED the reverted transport
	// gate: "reading your inbox is what keeps the seat live". A watch that does
	// not refresh does not hold up the thing it replaced.
	//
	// Rate-limited to the same cadence the attached sprint watch uses — a third
	// of the TTL, so two consecutive misses still leave the seat live — because
	// the poll tick can be a second and the sprint store is not something to
	// rewrite at that rate. This is the SAME refresher on a schedule, never a
	// second one: two liveness signals for one seat, disagreeing, is the defect
	// sprint 105 was opened to fix.
	lastBeat := poll.now()
	beatEvery := weave.SprintLeaseTTL / 3
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
		if now.Sub(lastBeat) >= beatEvery {
			// Best-effort and silent, exactly as at entry: bookkeeping the
			// caller did not ask for may never fail a read.
			refreshSprintOwnerActivity(reader)
			lastBeat = now
		}
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
					// The sync stamp (and any relay warning) is the one thing an
					// empty inbox must still say: "nothing new" from a host that
					// could not reach the relay is not the same as nothing new.
					for _, warning := range batch.warns {
						fmt.Fprintln(errOut, warning)
					}
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
// deliverSessionMail is the seam for the cross-host delivery pass; a test
// replaces it. nil means "no delivery on this host".
var deliverSessionMail = func(ctx context.Context, repoRoot, reader string) (weave.DeliveryReport, error) {
	return weave.DeliverRemoteMail(ctx, repoRoot, reader)
}

func snapshotUnifiedInbox(reader string, limit int, includeBus bool) (inboxBatch, error) {
	return snapshotInbox(reader, limit, includeBus, true)
}

// snapshotInbox is snapshotUnifiedInbox with the relay delivery pass optional:
// the per-command unread hint reads local stores only, so an ordinary command
// never waits on the network or files relay mail as a side effect.
func snapshotInbox(reader string, limit int, includeBus, deliver bool) (inboxBatch, error) {
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
			if state.Marks[fmt.Sprintf("%s:%d", idSource, event.Seq)].AckedAt != "" {
				continue
			}
			// A seeded Meet copy shares the original MB acknowledgment.
			if event.Origin != nil && strings.EqualFold(event.Origin.Source, "mb") &&
				state.Marks[fmt.Sprintf("mb:%d", event.Origin.Seq)].AckedAt != "" {
				continue
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
		mbEvents = append(mbEvents, unifiedInboxEvent{Schema: unifiedInboxSchema, Source: "mb", Seq: p.Seq, At: p.At, From: p.From, FromParty: p.FromParty, To: bus.RoleLabelFor(p.To), Topic: p.Topic, Body: p.Body})
	}
	appendLimited("mb", mbEvents, func() error { return bus.MarkSeen(reader, mbHigh) })

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
	for _, room := range rooms {
		if !stringMember(room.Members, reader) {
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
	}
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

// parseInboxSince accepts a look-back duration (2h, 30m, 3d) or an absolute
// RFC3339 / YYYY-MM-DD time.
func parseInboxSince(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasSuffix(raw, "d") {
		var days int
		if _, err := fmt.Sscanf(raw, "%dd", &days); err == nil && days >= 0 && fmt.Sprintf("%dd", days) == raw {
			return now.Add(-time.Duration(days) * 24 * time.Hour), nil
		}
	}
	if d, err := time.ParseDuration(raw); err == nil {
		if d < 0 {
			return time.Time{}, fmt.Errorf("inbox: --since %q must not be negative", raw)
		}
		return now.Add(-d), nil
	}
	if t, ok := parseInboxTime(raw); ok {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", raw, now.Location()); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("inbox: --since %q is not a duration (2h, 3d), RFC3339 time, or YYYY-MM-DD date", raw)
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

// inboxEventItem projects a record onto the mailbox item shape so the existing
// mailboxFilter selects it; the role source is "role:LABEL" in the stream and
// plain "role" to a filter.
func inboxEventItem(e unifiedInboxEvent) mailboxItem {
	source, _, _ := strings.Cut(e.Source, ":")
	return mailboxItem{Schema: mailboxSchema, Source: source, Seq: e.Seq, At: e.At, From: e.From, FromParty: e.FromParty, FromHandle: e.FromHandle, To: e.To, Topic: e.Topic, Room: e.Room, Body: e.Body}
}

// filteredInboxSnapshot narrows a read-only snapshot. It drops the batch's
// acknowledgements: a filtered view never moves a cursor past a record it hid.
// The cap applies to the filtered stream and keeps the newest records.
func filteredInboxSnapshot(next func(string, int, bool) (inboxBatch, error), f mailboxFilter, limit int) func(string, int, bool) (inboxBatch, error) {
	f.all = true
	return func(reader string, _ int, includeBus bool) (inboxBatch, error) {
		batch, err := next(reader, 0, includeBus)
		if err != nil {
			return batch, err
		}
		kept := batch.events[:0:0]
		for _, e := range batch.events {
			if f.match(inboxEventItem(e)) {
				kept = append(kept, e)
			}
		}
		if limit > 0 && len(kept) > limit {
			kept = kept[len(kept)-limit:]
		}
		batch.events = kept
		batch.acks = nil
		return batch, nil
	}
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
