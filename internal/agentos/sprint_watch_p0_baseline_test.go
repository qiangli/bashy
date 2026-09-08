package agentos

// Measurement oracle copied verbatim from Bashy 91d386f's watcher loop and
// acknowledgment lookup. Kept only in tests so the before/after experiment uses
// the same compiler, native notifier, fixture, and host with the original loop.
import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/qiangli/coreutils/pkg/room"
	"io"
	"strings"
	"time"
)

func runSprintInboxWatchP0Baseline(ctx context.Context, out, errOut io.Writer, sprintID int64,
	owner string, rt sprintWatchRuntime) error {
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
					Owner: owner, Attempt: misses,
					UnackedFor:  now.Sub(deliveredAt).Round(time.Second).String(),
					Instruction: fmt.Sprintf("you got message; after reading run `bashy sprint inbox-ack %d --as %s`", sprintID, owner),
				}
				if err := json.NewEncoder(out).Encode(reminder); err != nil {
					return err
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

func latestSprintWatchAckP0Baseline(id int64, owner string) (int64, error) {
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
