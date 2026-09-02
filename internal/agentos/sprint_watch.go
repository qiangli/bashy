package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/room"
	"github.com/qiangli/coreutils/pkg/weave"
	"github.com/spf13/cobra"
)

const (
	sprintWatchSchema      = "bashy-sprint-watch-v1"
	sprintWatchAckInterval = 3 * time.Minute
	sprintWatchMaxMisses   = 3
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
// own foreground tool call, so it dies with the harness; and it fails CLOSED on
// mail the manager does not acknowledge — three reminders and it exits. An
// unresponsive manager therefore stops heartbeating and its lease ages out
// normally, which is the property that makes refreshing from here honest.
var sprintWatchHeartbeat = weave.SprintLeaseTTL / 3

type sprintWatchReminder struct {
	Schema      string `json:"schema"`
	Type        string `json:"type"`
	Sprint      int64  `json:"sprint"`
	Owner       string `json:"owner"`
	Attempt     int    `json:"attempt"`
	MaxAttempts int    `json:"max_attempts"`
	UnackedFor  string `json:"unacked_for"`
	Instruction string `json:"instruction"`
}

type sprintWatchRuntime struct {
	ackEvery  time.Duration
	maxMisses int
	poll      inboxPollRuntime
	ackSeq    func(int64, string) (int64, error)
	// beatEvery and beat keep the attached seat's lease alive. beat is a var so
	// a test can drive the schedule without a sprint store on disk.
	beatEvery time.Duration
	beat      func(int64, string) error
}

func defaultSprintWatchRuntime() sprintWatchRuntime {
	return sprintWatchRuntime{
		ackEvery: sprintWatchAckInterval, maxMisses: sprintWatchMaxMisses,
		poll: defaultInboxPollRuntime(true), ackSeq: latestSprintWatchAck,
		beatEvery: sprintWatchHeartbeat, beat: weave.RefreshSprintManagerLease,
	}
}

// runSprintInboxWatch differs deliberately from ordinary inbox --watch: writing
// a pipe is not proof an external model consumed it. The source cursors remain
// untouched until the manager explicitly runs `sprint inbox-ack`; without that
// proof the watcher reminds three times and then fails closed.
func runSprintInboxWatch(ctx context.Context, out, errOut io.Writer, sprintID int64,
	owner string, rt sprintWatchRuntime) error {
	if rt.poll.close != nil {
		defer rt.poll.close()
	}
	gate := &inboxPollGate{reader: owner, fingerprint: rt.poll.fingerprint, fullRescan: rt.poll.fullRescan}
	interval := rt.poll.min
	var pending *inboxBatch
	var deliveredAt, nextReminder time.Time
	var ackBaseline int64
	misses := 0
	var nextBeat time.Time

	for {
		if rt.poll.ownerLive != nil {
			if err := rt.poll.ownerLive(); err != nil {
				return err
			}
		}
		now := rt.poll.now()
		// The attached stream IS the heartbeat: while this process runs, the
		// seat is held. A refusal here means the lease is no longer this
		// owner's — somebody took the seat over — and the honest response is to
		// stop, not to keep streaming mail addressed to a seat we have lost.
		if rt.beat != nil && (nextBeat.IsZero() || !now.Before(nextBeat)) {
			if err := rt.beat(sprintID, owner); err != nil {
				return fmt.Errorf("sprint watch: %s no longer holds sprint #%d — detaching: %w", owner, sprintID, err)
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
					return err
				}
				gate.commit(sum, sampled, now)
				if len(batch.events) == 0 {
					// Filtered outbound records are not manager input and need no
					// human/model acknowledgement to advance their exact watermark.
					if len(batch.acks) > 0 {
						if err := acknowledgeInboxBatch(batch); err != nil {
							return err
						}
					}
				} else {
					baseline, err := rt.ackSeq(sprintID, owner)
					if err != nil {
						return err
					}
					if err := renderInboxBatch(out, errOut, batch, true); err != nil {
						return err
					}
					pending = &batch
					ackBaseline = baseline
					deliveredAt = now
					nextReminder = now.Add(rt.ackEvery)
					misses = 0
				}
			}
		} else {
			seq, err := rt.ackSeq(sprintID, owner)
			if err != nil {
				return err
			}
			if seq > ackBaseline {
				if err := acknowledgeInboxBatch(*pending); err != nil {
					return err
				}
				pending = nil
				interval = rt.poll.min
				continue
			}
			if !now.Before(nextReminder) {
				misses++
				reminder := sprintWatchReminder{
					Schema: sprintWatchSchema, Type: "unacknowledged-inbox", Sprint: sprintID,
					Owner: owner, Attempt: misses, MaxAttempts: rt.maxMisses,
					UnackedFor:  now.Sub(deliveredAt).Round(time.Second).String(),
					Instruction: fmt.Sprintf("you got message; after reading run `bashy sprint inbox-ack %d --as %s`", sprintID, owner),
				}
				if err := json.NewEncoder(out).Encode(reminder); err != nil {
					return err
				}
				if misses >= rt.maxMisses {
					return fmt.Errorf("sprint watch: monitoring ENDED with error: %s did not acknowledge inbox input after %d reminders (%s); messages remain unread; rerun `bashy sprint take %d --as %s --watch`",
						owner, misses, now.Sub(deliveredAt).Round(time.Second), sprintID, owner)
				}
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
		if err := rt.poll.wait(ctx, pause); err != nil {
			return nil
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

func latestSprintWatchAck(id int64, owner string) (int64, error) {
	events, err := room.Timeline(0)
	if err != nil {
		return 0, err
	}
	var latest int64
	for _, event := range events {
		if event.Type == room.EventAck && event.Topic == sprintWatchTopic(id) &&
			strings.EqualFold(event.Actor, owner) && event.Target == room.AgentClaimID(owner) && event.Seq > latest {
			latest = event.Seq
		}
	}
	return latest, nil
}

func newSprintInboxAckCmd() *cobra.Command {
	var as string
	cmd := &cobra.Command{
		Use:   "inbox-ack <sprint>",
		Short: "Confirm that an external sprint manager read its attached inbox batch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSprintID(args[0])
			if err != nil {
				return err
			}
			owner, err := weave.SprintClaimIdentity(id, as, true)
			if err != nil {
				return err
			}
			card, live, err := room.Find(room.AgentClaimID(owner))
			if err != nil || !live || card.Mode != "sprint-inbox" || !room.HasCapability(card, room.CapInboxStream) {
				return fmt.Errorf("sprint inbox-ack: %s has no live attached sprint watch", owner)
			}
			actor, err := bus.ResolveAuthoredActor(owner)
			if err != nil {
				return err
			}
			if err := weave.RefreshSprintManagerLease(id, owner); err != nil {
				return fmt.Errorf("sprint inbox-ack: %w", err)
			}
			if err := room.Emit(room.Event{Type: room.EventAck, Actor: actor,
				Target: room.AgentClaimID(owner), Topic: sprintWatchTopic(id), Body: "attached inbox batch read"}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sprint inbox-ack: sprint #%d inbox batch acknowledged by %s\n", id, owner)
			return nil
		},
	}
	cmd.Flags().StringVar(&as, "as", "", "exact sprint-manager identity")
	return cmd
}

func parseSprintID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("sprint must be a positive integer: %q", raw)
	}
	return id, nil
}
