package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

const (
	sprintWatchSchema      = "bashy-sprint-watch-v1"
	sprintWatchAckInterval = 3 * time.Minute
)

// sprintWatchHeartbeat is how often the ATTACHED watch refreshes the lease it
// is holding open. A third of the TTL, so two consecutive misses still leave
// the seat live — the usual margin for a heartbeat that must not flap.
//
// WHY THE WATCH HEARTBEATS AT ALL. Before this, the only thing that refreshed a
// sprint lease was `sprint inbox-ack` — so the seat stayed live only while
// somebody kept sending the manager mail. A conductor working steadily and
// receiving nothing went STALE in thirty minutes with its mandated watch still
// attached: measured at 2h08m of live watch against 1h19m of "STALE (no
// heartbeat — take it)", on a seat the same board simultaneously marked live.
// Two liveness signals for one seat, disagreeing.
//
// It is not a timer pretending to be evidence. This process is the harness's
// own foreground tool call, so it dies with the harness. Unacknowledged mail
// remains durable and reminders continue until the harness acknowledges it;
// unread input must never destroy the delivery path that can handle it.
var sprintWatchHeartbeat = weave.SprintLeaseTTL / 3

type sprintWatchReminder struct {
	Schema      string `json:"schema"`
	Type        string `json:"type"`
	Sprint      int64  `json:"sprint"`
	Owner       string `json:"owner"`
	Attempt     int    `json:"attempt"`
	UnackedFor  string `json:"unacked_for"`
	Instruction string `json:"instruction"`
}

type sprintWatchRuntime struct {
	observe      func(context.Context, int64, string)
	observeEvery time.Duration
	ackEvery     time.Duration
	poll         inboxPollRuntime
	ackSeq       func(int64, string) (int64, error) // legacy test injection
	ackPosition  func(context.Context, int64, string) (room.TimelinePosition, error)
	// beatEvery and beat keep the attached seat's lease alive; release stands
	// it back down when this stream detaches. Both are vars so a test can
	// drive the schedule without a sprint store on disk.
	beatEvery time.Duration
	beat      func(int64, string) error
	release   func(int64, string) error
}

func defaultSprintWatchRuntime() sprintWatchRuntime {
	return sprintWatchRuntime{
		observe: observeSprintResources, observeEvery: 5 * time.Second,
		ackEvery: sprintWatchAckInterval,
		poll:     defaultInboxPollRuntime(true), ackPosition: newSprintWatchAckReader().latest,
		beatEvery: sprintWatchHeartbeat, beat: holdSprintWatchLease,
		release: weave.ReleaseSprintManagerLease,
	}
}

// holdSprintWatchLease beats the lease AND stamps this process onto it. The
// attached watch is the one refresher that is still running when its beat is
// read back, so it is the one that can be checked — see
// sprintConductorHealth.
func holdSprintWatchLease(id int64, owner string) error {
	return weave.HoldSprintManagerLease(id, owner, os.Getpid())
}

// runSprintInboxWatch differs deliberately from ordinary inbox --watch: writing
// a pipe is not proof an external model consumed it. The source cursors remain
// untouched until the manager explicitly runs `sprint inbox-ack`; without that
// proof the watcher keeps reminding while leaving unread input durable.
// EVERY EXIT IS EXPLAINED.
//
// Sprint 412's mandated watch exited and said nothing (todo 4a18c41ef997), and
// the manager's next evidence was a board reporting STALE. The exit status was
// no help either way: this loop ends with a nil error on its NORMAL paths —
// ctrl-C, SIGTERM, the harness exiting, a cancelled context — so a conductor
// whose seat had just been stood down saw a clean `exit 0` and no reason at
// all, exactly as uninformative as the silent failure.
//
// So every way out writes one line naming the cause and what it means for the
// mail. It goes to STDERR because stdout carries the NDJSON a reader parses,
// and it is written AFTER the loop's own defers so it is the last word of the
// stream rather than a note the release might outlive.
func runSprintInboxWatch(ctx context.Context, out, errOut io.Writer, sprintID int64,
	owner string, rt sprintWatchRuntime) error {
	cause, err := runSprintInboxWatchLoop(ctx, out, errOut, sprintID, owner, rt)
	writeSprintWatchDetach(errOut, sprintID, owner, cause, err)
	return err
}

// sprintWatchDetachCause names why the loop below returned, in the reader's
// terms. The zero value means it fell out without one of the known causes,
// which is itself worth saying rather than hiding.
type sprintWatchDetachCause string

const (
	sprintWatchDetachCancelled sprintWatchDetachCause = "the context was cancelled — an interrupt, a SIGTERM, or the harness that owns this process exiting"
	sprintWatchDetachOwnerGone sprintWatchDetachCause = "the session this watch was registered under is no longer running"
	sprintWatchDetachTakeover  sprintWatchDetachCause = "the sprint lease is no longer held by this owner — the seat was taken over"
	sprintWatchDetachStore     sprintWatchDetachCause = "a store read or write failed"
	sprintWatchDetachRender    sprintWatchDetachCause = "the stream could not be written — the reader is gone or the pipe closed"
)

// writeSprintWatchDetach is the last line of an attached watch.
//
// It always states the mail position, because that is the question an agent
// actually has when its stream ends and the one it most often gets wrong:
// unacknowledged input is never consumed, so nothing is lost by a detach, and
// the same mail is still there to read. Saying so here is what stops a
// recovering manager from assuming a dropped batch.
func writeSprintWatchDetach(w io.Writer, sprintID int64, owner string, cause sprintWatchDetachCause, err error) {
	if w == nil {
		return
	}
	reason := string(cause)
	if reason == "" {
		reason = "the watch loop ended without naming a cause"
	}
	fmt.Fprintf(w, "sprint watch: detached from sprint #%d as %s: %s\n", sprintID, owner, reason)
	if err != nil {
		fmt.Fprintf(w, "sprint watch: the seat is no longer held by this process; `bashy sprint show %d` reports it as unheld once the lease expires\n", sprintID)
	}
	fmt.Fprintf(w, "sprint watch: NO MAIL WAS LOST — unacknowledged input is never consumed. Read it with `bashy inbox --as %s` (or reattach with `bashy sprint take %d --owner %s --watch`)\n", owner, sprintID, owner)
}

func runSprintInboxWatchLoop(ctx context.Context, out, errOut io.Writer, sprintID int64,
	owner string, rt sprintWatchRuntime) (sprintWatchDetachCause, error) {
	if rt.poll.close != nil {
		defer rt.poll.close()
	}
	// DETACHING IS AN EVENT AND HAS TO BE WRITTEN DOWN. This stream is what
	// holds the seat; every way out of the loop below — ctrl-C, a cancelled
	// context, a store error, the owner's harness exiting — ends the only
	// thing standing behind the last beat. Left unwritten, that beat kept the
	// board reporting a healthy conductor for the rest of the TTL. Silent
	// because it is bookkeeping: a watch must not fail on its way out over the
	// note it wrote about leaving, and the holder-check inside release already
	// makes it a no-op when somebody else has taken the seat.
	if rt.release != nil {
		defer func() { _ = rt.release(sprintID, owner) }()
	}
	stopObservation := startSprintObservation(ctx, sprintID, owner, rt.observeEvery, rt.observe)
	defer stopObservation()
	gate := &inboxPollGate{reader: owner, fingerprint: rt.poll.fingerprint, fullRescan: rt.poll.fullRescan}
	interval := rt.poll.min
	var pending *inboxBatch
	var deliveredAt, nextReminder time.Time
	var ackBaseline room.TimelinePosition
	ackPosition := rt.ackPosition
	if ackPosition == nil {
		ackPosition = func(_ context.Context, id int64, owner string) (room.TimelinePosition, error) {
			seq, err := rt.ackSeq(id, owner)
			return room.TimelinePosition{Generation: 1, Seq: seq}, err
		}
	}
	misses := 0
	var nextBeat time.Time

	for {
		if ctx.Err() != nil {
			return sprintWatchDetachCancelled, nil
		}
		if rt.poll.ownerLive != nil {
			if err := rt.poll.ownerLive(); err != nil {
				return sprintWatchDetachOwnerGone, err
			}
		}
		now := rt.poll.now()
		// The attached stream IS the heartbeat: while this process runs, the
		// seat is held. A refusal here means the lease is no longer this
		// owner's — somebody took the seat over — and the honest response is to
		// stop, not to keep streaming mail addressed to a seat we have lost.
		if rt.beat != nil && (nextBeat.IsZero() || !now.Before(nextBeat)) {
			if err := rt.beat(sprintID, owner); err != nil {
				return sprintWatchDetachTakeover,
					fmt.Errorf("sprint watch: %s no longer holds sprint #%d — detaching: %w", owner, sprintID, err)
			}
			beatEvery := rt.beatEvery
			if beatEvery <= 0 {
				beatEvery = sprintWatchHeartbeat
			}
			nextBeat = now.Add(beatEvery)
		}
		if pending == nil {
			read, changed, sum, sampled := gate.due(now)
			if changed {
				interval = rt.poll.min
			}
			if read {
				batch, err := rt.poll.snapshot(owner, 0, true)
				if err != nil {
					return sprintWatchDetachStore, fmt.Errorf("sprint watch: read the unified inbox for %s: %w", owner, err)
				}
				gate.commit(sum, sampled, now)
				if len(batch.events) == 0 {
					// Filtered outbound records are not manager input and need no
					// human/model acknowledgement to advance their exact watermark.
					if len(batch.acks) > 0 {
						if err := acknowledgeInboxBatch(batch); err != nil {
							return sprintWatchDetachStore,
								fmt.Errorf("sprint watch: advance the watermark past filtered outbound records: %w", err)
						}
					}
				} else {
					baseline, err := ackPosition(ctx, sprintID, owner)
					if err != nil {
						if ctx.Err() != nil {
							return sprintWatchDetachCancelled, nil
						}
						return sprintWatchDetachStore, fmt.Errorf("sprint watch: read the acknowledgement timeline: %w", err)
					}
					if err := renderInboxBatch(out, errOut, batch, true); err != nil {
						return sprintWatchDetachRender, fmt.Errorf("sprint watch: write the delivered batch: %w", err)
					}
					pending = &batch
					ackBaseline = baseline
					deliveredAt = now
					nextReminder = now.Add(rt.ackEvery)
					misses = 0
				}
			}
		} else {
			position, err := ackPosition(ctx, sprintID, owner)
			if err != nil {
				if ctx.Err() != nil {
					return sprintWatchDetachCancelled, nil
				}
				return sprintWatchDetachStore, fmt.Errorf("sprint watch: read the acknowledgement timeline: %w", err)
			}
			if position.Generation != ackBaseline.Generation {
				// A replaced stream contains historical input. Rebase and require a
				// subsequent explicit ack rather than consuming mail on replay.
				ackBaseline = position
			} else if position.Seq > ackBaseline.Seq {
				if err := acknowledgeInboxBatch(*pending); err != nil {
					return sprintWatchDetachStore,
						fmt.Errorf("sprint watch: consume the acknowledged batch: %w", err)
				}
				pending = nil
				interval = rt.poll.min
				continue
			}
			if !now.Before(nextReminder) {
				misses++
				reminder := sprintWatchReminder{
					Schema: sprintWatchSchema, Type: "unacknowledged-inbox", Sprint: sprintID,
					Owner: owner, Attempt: misses,
					UnackedFor:  now.Sub(deliveredAt).Round(time.Second).String(),
					Instruction: fmt.Sprintf("you got message; after reading run `bashy sprint inbox-ack %d --as %s`", sprintID, owner),
				}
				if err := json.NewEncoder(out).Encode(reminder); err != nil {
					return sprintWatchDetachRender, fmt.Errorf("sprint watch: write the unacknowledged-inbox reminder: %w", err)
				}
				// IT KEEPS REMINDING; IT DOES NOT QUIT.
				//
				// The guard that prevents message loss is that a cursor never
				// advances until the manager proves it read — and that holds
				// whether or not this process is alive. Exiting protected
				// nothing further: the mail was already safe. What it did do
				// was destroy the seat's delivery path over an unacknowledged
				// message, so a conductor that was merely busy came back to a
				// dead watch and a board reporting UNREACHABLE. Measured twice
				// in one session on this project's own sprint.
				//
				// So the reminder stays (the sender is waiting and that must be
				// visible) and the fuse goes. Unread mail is already reported
				// where it belongs — `sprint show` counts unanswered messages
				// and names the oldest.
				nextReminder = nextReminder.Add(rt.ackEvery)
			}
		}

		pause := interval
		if pending != nil && now.Add(pause).After(nextReminder) {
			pause = nextReminder.Sub(now)
		}
		if pause <= 0 {
			pause = rt.poll.min
		}
		waitStarted := time.Now()
		if err := rt.poll.wait(ctx, pause); err != nil {
			return sprintWatchDetachCancelled, nil
		}
		// Native notifications can concern unrelated files (including lock
		// diagnostics). Coalesce those wakes while a batch awaits its ack.
		if pending != nil {
			if remaining := pause - time.Since(waitStarted); remaining > 0 {
				if err := waitInboxPoll(ctx, remaining); err != nil {
					return sprintWatchDetachCancelled, nil
				}
			}
		}
		if pending == nil {
			interval *= 2
			if interval > rt.poll.max {
				interval = rt.poll.max
			}
		}
	}
}

func sprintWatchTopic(id int64) string { return fmt.Sprintf("sprint.%d.inbox-read", id) }

// Each watch owns its reduction; switching identity discards any prior stream.
type sprintWatchAckReader struct {
	reader *room.TimelineReader
	id     int64
	owner  string
}

func newSprintWatchAckReader() *sprintWatchAckReader { return &sprintWatchAckReader{} }
func (r *sprintWatchAckReader) latest(ctx context.Context, id int64, owner string) (room.TimelinePosition, error) {
	if r.reader == nil || r.id != id || r.owner != owner {
		r.id = id
		r.owner = owner
		r.reader = room.NewTimelineReader(func(event room.Event) bool {
			return event.Type == room.EventAck && event.Topic == sprintWatchTopic(id) &&
				strings.EqualFold(event.Actor, owner) && event.Target == room.AgentClaimID(owner)
		})
	}
	return r.reader.Latest(ctx)
}

func newSprintInboxAckCmd() *cobra.Command {
	var as string
	cmd := &cobra.Command{
		Use:   "inbox-ack <sprint>",
		Short: "Confirm that an external sprint manager read its attached inbox batch",
		Long: `inbox-ack is the ACK half of ` + "`sprint take/start <id> --owner NAME --watch`" + `.

It exists because writing a pipe is not proof a model read it. The attached
watch renders each batch and then leaves every source cursor untouched until
this command proves the manager consumed it; without that proof the watch keeps
reminding and the mail stays unread. So inbox-ack is meaningful ONLY against a
live attached watch — it acknowledges THAT stream's batch, it is not itself a
read and it consumes nothing on its own.

THE CONSUMING READS, by name — if you have no attached watch, this command is
not what you want and will refuse:

  bashy inbox --as NAME                  read and consume pending mail now
  bashy inbox --as NAME --watch --wait 5s  same, waiting briefly for arrivals
  bashy inbox --as NAME --peek           look without consuming
  bashy inbox ack --as NAME <id>…        consume named records only

Those advance the cursors. inbox-ack does not; it records that the attached
stream's batch was handled and refreshes the sprint lease the stream holds.

A refusal NEVER consumes or discards anything: the unread records are exactly
as they were, and the refusal names which read will consume them.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSprintID(args[0])
			if err != nil {
				return sprintUsage(err)
			}
			owner, err := weave.SprintClaimIdentity(id, as, true)
			if err != nil {
				return sprintUsage(err)
			}
			if err := requireAttachedSprintWatch(id, owner); err != nil {
				return err
			}
			actor, err := bus.ResolveAuthoredActor(owner)
			if err != nil {
				return sprintUsage(err)
			}
			if err := weave.RefreshSprintManagerLease(id, owner); err != nil {
				return fmt.Errorf("refresh the sprint lease this ack proves activity on: %w", err)
			}
			if err := room.Emit(room.Event{Type: room.EventAck, Actor: actor,
				Target: room.AgentClaimID(owner), Topic: sprintWatchTopic(id), Body: "attached inbox batch read"}); err != nil {
				return fmt.Errorf("record the acknowledgement: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sprint inbox-ack: sprint #%d inbox batch acknowledged by %s\n", id, owner)
			return nil
		},
	}
	cmd.Flags().StringVar(&as, "as", "", "exact sprint-manager identity")
	return cmd
}

// requireAttachedSprintWatch is the precondition inbox-ack refuses on, and the
// refusal has to be ACTIONABLE because this is where an external manager gets
// stuck. Sprint 329's manager had 14 unread messages, ran
// `bashy sprint inbox-ack 329 --as s329-manager`, got exit 1 and NOTHING on
// either stream, and `sprint tick` still said 14 unread (todo 491a03436cbb).
// The silence was the host's (see sprint_errors.go), but the message underneath
// it was no better: one sentence, four different causes collapsed into it, and
// no mention of the command that would actually consume the mail — the manager
// found `bashy inbox --as s329-manager --watch` by guessing.
//
// So each cause is named separately and every refusal ends in the read that
// consumes. room.Find's own error is reported instead of being folded into "no
// live watch": a store that cannot be read is not the same fact as a watch that
// is not there, and treating them alike sends the reader to fix the wrong thing.
//
// NOTHING here mutates: a refusal must leave every unread record exactly where
// it was, which is the whole point of requiring an ack.
func requireAttachedSprintWatch(id int64, owner string) error {
	card, live, err := room.Find(room.AgentClaimID(owner))
	if err != nil {
		return fmt.Errorf("cannot read the presence record for %s: %w; nothing was acknowledged and no mail was consumed", owner, err)
	}
	switch {
	case !live:
		return sprintUsagef("%[1]s has no live attached sprint watch, so there is no delivered batch to acknowledge (nothing was consumed; unread mail is untouched).\n"+
			"  to acknowledge:  start one first — `bashy sprint take %[2]d --owner %[1]s --watch`, retain that process, then ack each batch it renders\n"+
			"  to just READ the mail now, which DOES consume it:  `bashy inbox --as %[1]s --watch --wait 5s` (or `bashy inbox --as %[1]s`)\n"+
			"  to look without consuming:  `bashy inbox --as %[1]s --peek`", owner, id)
	case !strings.EqualFold(card.Mode, "sprint-inbox") || !room.HasCapability(card, room.CapInboxStream):
		mode := strings.TrimSpace(card.Mode)
		if mode == "" {
			mode = "unknown"
		}
		return sprintUsagef("%[1]s is live as %[3]q, not as an attached sprint watch, so it has no sprint batch to acknowledge (nothing was consumed; unread mail is untouched).\n"+
			"  a plain `bashy inbox --watch` consumes mail by itself and needs no ack — if that is the session you are driving, you are already done\n"+
			"  for the sprint seat:  `bashy sprint take %[2]d --owner %[1]s --watch` (one watcher per identity — stop the other reader first)\n"+
			"  to read the mail now, which DOES consume it:  `bashy inbox --as %[1]s --watch --wait 5s`", owner, id, mode)
	}
	return nil
}

func parseSprintID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("sprint must be a positive integer: %q", raw)
	}
	return id, nil
}
