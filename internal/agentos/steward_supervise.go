// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/chat"
	"github.com/qiangli/coreutils/pkg/handoff"
	"github.com/qiangli/coreutils/pkg/steward"
)

// The supervisor is the process that IS the background steward.
//
// It holds three things the agent cannot hold for itself, and it is worth being
// precise about why each one needs a process rather than an instruction:
//
//	LIVENESS   the seat lapses without a heartbeat, and an agent deep in a
//	           twenty-minute investigation is not going to remember to send one.
//	           A lapsed seat is claimable, so forgetting costs the tenure.
//	REACH      an agent in a turn cannot decide to go and look at a channel. The
//	           supervisor watches the board and the seat inbox and NUDGES at a
//	           turn boundary — the one moment the agent can act on it.
//	AN ENDING  a steward that is killed leaves no note. The wrap-up sequence
//	           below is the only thing standing between "stopped" and "vanished".
//
// It is deliberately NOT the agent's brain. It never answers mail, never decides
// what matters, and never writes to the journal on the agent's behalf. It counts,
// points, and gets out of the way.

const (
	stewardHeartbeatEvery = 60 * time.Second
	stewardMailPollEvery  = 20 * time.Second
	stewardIdleQuiet      = 25 * time.Second
)

// runStewardSupervisor is the detached child's whole life. It returns only when
// the session ends or a stop is requested.
func runStewardSupervisor(ctx context.Context, errW io.Writer, opt stewardStartOptions) error {
	sess, err := loadStewardSession()
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("steward supervise: no session record — `steward start` writes it before spawning this process")
	}

	// The stop signal. SIGTERM is how `steward stop` asks; SIGINT is how a human
	// at the terminal asks. Both mean the same thing here: wrap up.
	ctx, stopSignals := signal.NotifyContext(ctx, append([]os.Signal{os.Interrupt}, stewardTermSignals()...)...)
	defer stopSignals()

	brief, state := stewardBootstrapBrief(sess, opt)
	sess.Handoff = state
	_ = saveStewardSession(sess)

	fmt.Fprintf(errW, "steward: launching %s (%s) as the host steward\n", sess.Agent, sess.Binding)

	agentSession, err := chat.Start(ctx, sess.Agent, chat.SessionOptions{
		Prompt: brief,
		Cwd:    sess.Cwd,
		// Mode labels the host-room card, so `bashy chat sessions` shows WHAT
		// this agent is — the difference between "an agent is running" and "the
		// steward is running".
		Mode: "steward",
		// NOT Attended. No human is driving this turn by turn; the room, the
		// bus and `chat attach` are the supervision. Marking it attended would
		// claim an oversight that is not there.
		Attended:     false,
		ReadOnly:     false,
		AllowPremium: opt.AllowPremium,
		AllowUnsafe:  opt.Yolo,
	})
	if err != nil {
		_ = clearStewardSession()
		return fmt.Errorf("steward: the agent would not start: %w", err)
	}
	defer agentSession.Close()

	fmt.Fprintf(errW, "steward: %s is live; reachable on bus %s\n", sess.Agent, sess.Topic)

	// THE SIDECAR. It is the existing bus sidecar, hosted here rather than
	// reimplemented — see stewardRunSidecar for why this process is where it
	// belongs.
	if opt.Sidecar {
		go stewardRunSidecar(ctx, errW, sess)
	}

	stopReason := stewardWatch(ctx, errW, agentSession, sess, opt)

	return stewardWrapUp(errW, agentSession, sess, opt, stopReason)
}

// stewardWatch is the steady state: heartbeat the seat, nudge on new mail, and
// notice if the agent dies. It returns why it stopped.
func stewardWatch(ctx context.Context, errW io.Writer, s *chat.Session, sess *stewardSession, opt stewardStartOptions) string {
	heartbeat := time.NewTicker(stewardHeartbeatEvery)
	defer heartbeat.Stop()
	mail := time.NewTicker(stewardMailPollEvery)
	defer mail.Stop()

	watch := &stewardMailWatch{topic: sess.Topic, subscriber: stewardSubscriberName()}
	// Prime the cursors so the first tick reports what arrives FROM NOW, not the
	// entire backlog — the backlog is in the bootstrap brief, which told the
	// agent to read it properly rather than have it pasted at them.
	watch.prime()

	mediator, why := (*stewardMediator)(nil), ""
	if opt.Mediator {
		mediator, why = resolveStewardMediator(opt.MediatorAgent, opt.MediatorBand, sess.Band)
		if mediator != nil {
			fmt.Fprintf(errW, "steward: mediator %s (%s, L%d) will triage new mail\n", mediator.Agent, mediator.Binding, mediator.Band)
		} else {
			// Say WHY there is no mediator. Silence here would leave an operator
			// who asked for cheap triage believing they got it.
			fmt.Fprintf(errW, "steward: %s — mail notices stay mechanical (free)\n", why)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return "stop requested"
		case <-heartbeat.C:
			if sess.SeatHeld && sess.Epoch > 0 {
				if err := stewardHeartbeat(sess.Epoch); err != nil {
					// A FENCED HEARTBEAT IS THE END OF THE TENURE, not a
					// transient error: somebody took the seat over. Say so and
					// stand down rather than keep running as a steward the host
					// no longer recognises.
					fmt.Fprintf(errW, "steward: heartbeat rejected (%v) — the seat has moved on; standing down\n", err)
					return "fenced: the seat was taken over"
				}
			}
			if !s.Live() {
				return "the agent exited"
			}
		case <-mail.C:
			if !s.Live() {
				return "the agent exited"
			}
			notice, newSeat, newBoard := watch.poll()
			if notice == "" {
				continue
			}
			// TRIAGE BEFORE WAITING, not after. The mediator call is a separate
			// cheap process; running it while the steward is still finishing its
			// turn overlaps the two instead of serialising them, and a digest
			// that arrives late is no worse than one that arrives early.
			if mediator != nil {
				if digest, why := mediator.mediate(ctx, newSeat, newBoard); digest != "" {
					notice = notice + "\n" + digest
				} else if why != "" {
					// A DEGRADED TRIAGE IS ANNOUNCED. The steward must know it
					// is reading the raw pointer rather than a digest, or it
					// will read the absence of urgent lines as the absence of
					// urgency.
					fmt.Fprintf(errW, "steward: %s\n", why)
					notice = notice + "\n[bashy] triage unavailable (" + why + ") — the counts above are all that is known."
				}
			}
			// AT A TURN BOUNDARY, NEVER MID-TURN. A message injected while the
			// agent is thinking is either swallowed or corrupts the turn, which
			// is the exact failure the bus sidecar was built to avoid. Waiting
			// for idle costs latency and buys delivery.
			wctx, cancel := context.WithTimeout(ctx, opt.NudgeWait)
			err := s.WaitIdle(wctx, stewardIdleQuiet)
			cancel()
			if err != nil {
				// Still busy. Keep the counts; the next tick tries again.
				continue
			}
			watch.commit()
			if err := s.Say(notice); err != nil {
				fmt.Fprintf(errW, "steward: could not deliver a mail notice (%v)\n", err)
			}
		}
	}
}

// stewardWrapUp is the ending: ask for the note, verify one arrived, close the
// room, release the seat.
//
// EVERY STEP REPORTS WHAT ACTUALLY HAPPENED. A stop that closed the room but
// never got a note is a different event from a clean handover, and the outcome
// file says which one it was — because the next steward reads the note, and a
// note that was never written must not look like one that was.
func stewardWrapUp(errW io.Writer, s *chat.Session, sess *stewardSession, opt stewardStartOptions, reason string) error {
	out := &stewardStopOutcome{Agent: sess.Agent, StoppedAt: time.Now().UTC(), Detail: reason}
	since := time.Now().UTC().Add(-time.Second)

	fmt.Fprintf(errW, "steward: stopping (%s) — asking %s for a handoff note\n", reason, sess.Agent)

	if s.Live() && !opt.NoNote {
		ctx, cancel := context.WithTimeout(context.Background(), opt.StopTimeout)
		if err := s.Say(stewardWrapUpInstruction(sess)); err != nil {
			fmt.Fprintf(errW, "steward: could not ask for a note (%v)\n", err)
		} else if err := s.WaitIdle(ctx, stewardIdleQuiet); err != nil {
			fmt.Fprintf(errW, "steward: the note request did not finish within %s (%v)\n", opt.StopTimeout, err)
		}
		cancel()
	}

	// DID A NOTE ACTUALLY LAND? Asking is not the same as being answered, and
	// the agent's own "done" is a claim. Look in the store.
	if rec := newestStewardNoteSince(since); rec != nil {
		out.NoteWritten, out.NoteID, out.NoteBy = true, rec.ID, "agent"
		out.NoteDetail = stewardFirstLine(rec.Continuity)
	} else if !opt.NoNote {
		// The fallback. It is NOT a briefing and never pretends to be — it is a
		// pointer at the journal saying "the steward stopped here and did not
		// leave a note", so the next steward reconciles instead of assuming a
		// clean start.
		if rec, err := writeStewardFallbackNote(sess, reason); err != nil {
			out.NoteDetail = fmt.Sprintf("no note, and the fallback could not be written: %v", err)
			fmt.Fprintf(errW, "steward: %s\n", out.NoteDetail)
		} else {
			out.NoteWritten, out.NoteID, out.NoteBy = true, rec.ID, "fallback"
			out.NoteDetail = "the agent produced no note; bashy recorded a MECHANICAL one — a pointer at the journal, not a briefing"
			fmt.Fprintf(errW, "steward: %s\n", out.NoteDetail)
		}
	}

	_ = s.Quit()

	// The room closes before the seat is released, so there is never a moment
	// where the host advertises a channel to a seat nobody holds.
	if line := steward.ReleaseRoom(steward.HolderName()); line != "" {
		out.RoomClosed = !strings.Contains(line, "could not be closed")
		fmt.Fprint(errW, line)
	}

	if sess.SeatHeld && sess.Epoch > 0 && !opt.KeepSeat {
		if err := stewardRelease(sess.Epoch, "steward stop: "+reason); err != nil {
			fmt.Fprintf(errW, "steward: the seat could not be released (%v) — it will lapse on its own\n", err)
		} else {
			out.SeatReleased = true
		}
	}

	if err := saveStewardOutcome(out); err != nil {
		fmt.Fprintf(errW, "steward: the stop outcome could not be recorded (%v)\n", err)
	}
	_ = clearStewardSession()
	fmt.Fprintln(errW, "steward: stopped.")
	return nil
}

// ─── the sidecar ──────────────────────────────────────────────────────────────

// stewardRunSidecar hosts the EXISTING bus sidecar in the steward's supervisor.
//
// It is not a new poller. bashy already has one — pkg/bus's Sidecar, which holds
// standing subscriptions, matches topics, applies the interrupt governance and
// the rate limit, and steers a live session over its control socket. What it has
// never had is a process to live in: a host with no long-running bashy has no
// sidecar, so `bus subscribe --interrupt-from steward` describes a delivery tier
// that nothing performs.
//
// The steward supervisor is the natural owner. It is the one process a stewarded
// host keeps up for as long as it is attended, and it is already the thing whose
// agent the sidecar would be interrupting. Hosting it here costs nothing (the
// sidecar is mechanical — no model, no tokens) and gives the queued/interrupt
// split its first real implementation on this machine.
func stewardRunSidecar(ctx context.Context, errW io.Writer, sess *stewardSession) {
	// Register the seat's standing interest FIRST, so the sidecar has something
	// to resolve against. The seat — not the agent — is the subscriber: mail
	// addressed to the steward must survive this agent being replaced, which is
	// the whole reason the bus addresses a role.
	sub := bus.Subscription{
		Subscriber: sess.Topic,
		To:         sess.Topic,
		Instance:   sess.Agent,
		// The steward is the host's escalation point, so an interrupt from a
		// conductor is exactly the message it must not read twenty minutes
		// late. Everything else queues, which is the default and stays it.
		InterruptFrom: []string{"conductor", "human", "operator"},
	}
	if err := bus.SaveSubscription(sub); err != nil {
		fmt.Fprintf(errW, "steward: the seat's bus subscription could not be saved (%v) — mail will still queue, but nothing can interrupt a turn\n", err)
	}

	sc := bus.NewSidecar(0)
	fmt.Fprintf(errW, "steward: bus sidecar watching (poll %s)\n", sc.Poll)
	if err := sc.Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(errW, "steward: the bus sidecar stopped (%v) — mail still resolves on read, only interrupts are lost\n", err)
	}
}

// stewardMailWatch counts what has arrived on the two channels a steward is
// addressable on, WITHOUT reading either of them.
//
// Not reading is the point, and it is the one design decision here worth
// defending. Both stores MARK ON READ. If the supervisor drained the mail and
// pasted the bodies into the agent's prompt, the inbox would show every message
// read — by a process that is not the steward — while the steward's own record
// showed it never looked. The channel would report a healthy read side and the
// human would have no way to tell that nobody had actually considered anything.
//
// So the supervisor delivers a POINTER: how many, from where, go and look. The
// agent runs `bashy steward inbox` / `bashy mb` itself, the read is attributed
// to the steward, and the bodies never bypass the verbs that record them.
type stewardMailWatch struct {
	topic      string
	subscriber string

	seenSeat  int64
	seenBoard int64
	// pending* hold the counts a poll found, promoted to seen only once the
	// notice is actually DELIVERED. A nudge that could not be delivered must not
	// mark the mail as announced.
	pendSeat  int64
	pendBoard int64
	nSeat     int
	nBoard    int
	newSeat   []bus.Pending
	newBoard  []bus.Pending
}

func (w *stewardMailWatch) prime() {
	w.seenSeat = maxSeq(peekSeat(w.topic))
	w.seenBoard = maxSeq(peekBoard(w.subscriber))
}

// poll returns the mechanical notice plus the NEW items on each channel (so the
// mediator, if there is one, has something to triage). An empty notice means
// nothing arrived.
func (w *stewardMailWatch) poll() (notice string, newSeat, newBoard []bus.Pending) {
	seat := peekSeat(w.topic)
	board := peekBoard(w.subscriber)

	w.newSeat, w.pendSeat = newerThan(seat, w.seenSeat)
	w.newBoard, w.pendBoard = newerThan(board, w.seenBoard)
	w.nSeat, w.nBoard = len(w.newSeat), len(w.newBoard)
	return w.render()
}

// render turns the counted state into the notice the agent is sent. Split from
// poll so the WORDING can be tested without a live bus — the wording is the
// part that carries the guarantee.
func (w *stewardMailWatch) render() (notice string, newSeat, newBoard []bus.Pending) {
	if w.nSeat == 0 && w.nBoard == 0 {
		return "", nil, nil
	}

	// THE TWO CHANNELS ARE REPORTED SEPARATELY, always. The seat inbox is mail
	// addressed to the ROLE — it includes what predecessors were sent and never
	// answered. The board is the host's public channel. Collapsing them into
	// "you have 4 messages" loses the only distinction that decides whether a
	// predecessor's unanswered mail is being read.
	var parts []string
	if w.nSeat > 0 {
		parts = append(parts, fmt.Sprintf("%d new at the SEAT inbox (addressed to the steward role) — `bashy steward inbox`", w.nSeat))
	}
	if w.nBoard > 0 {
		parts = append(parts, fmt.Sprintf("%d new on the MESSAGE BOARD — `bashy mb`", w.nBoard))
	}
	return "[bashy] " + strings.Join(parts, "; ") +
			". Read them yourself — this notice is a pointer, not the mail, and the read is only recorded when you look.",
		w.newSeat, w.newBoard
}

// commit promotes the last poll's cursors. Called only after the notice was
// delivered.
func (w *stewardMailWatch) commit() {
	if w.pendSeat > w.seenSeat {
		w.seenSeat = w.pendSeat
	}
	if w.pendBoard > w.seenBoard {
		w.seenBoard = w.pendBoard
	}
}

// peekSeat/peekBoard read WITHOUT marking. An error is an empty read: a watcher
// that cannot see the mail must not invent it, and the agent's own verbs will
// report the failure honestly when it goes to look.
func peekSeat(topic string) []bus.Pending {
	if strings.TrimSpace(topic) == "" {
		return nil
	}
	items, err := bus.SeatPending(topic, true, false)
	if err != nil {
		return nil
	}
	return items
}

func peekBoard(subscriber string) []bus.Pending {
	if strings.TrimSpace(subscriber) == "" {
		return nil
	}
	items, err := bus.UnreadPending(subscriber)
	if err != nil {
		return nil
	}
	return items
}

func maxSeq(items []bus.Pending) int64 {
	var m int64
	for _, it := range items {
		if it.Seq > m {
			m = it.Seq
		}
	}
	return m
}

func newerThan(items []bus.Pending, since int64) (out []bus.Pending, high int64) {
	high = since
	for _, it := range items {
		if it.Seq > since {
			out = append(out, it)
			if it.Seq > high {
				high = it.Seq
			}
		}
	}
	return out, high
}

func stewardSubscriberName() string {
	r := steward.Self()
	if r.Name != "" {
		return r.Name
	}
	return "steward"
}

// ─── seat writes ──────────────────────────────────────────────────────────────

func stewardHeartbeat(epoch uint64) error {
	st, err := steward.Open("")
	if err != nil {
		return err
	}
	return st.Heartbeat(steward.Self(), epoch, time.Now())
}

func stewardRelease(epoch uint64, note string) error {
	st, err := steward.Open("")
	if err != nil {
		return err
	}
	return st.Release(steward.Self(), epoch, note, time.Now())
}

// ─── the fallback note ────────────────────────────────────────────────────────

// newestStewardNoteSince reports a steward-role handoff written after t — the
// evidence that the agent actually answered the wrap-up request.
func newestStewardNoteSince(t time.Time) *handoff.Record {
	recs, err := handoff.List(handoff.DefaultDir())
	if err != nil {
		return nil
	}
	var best *handoff.Record
	for _, r := range recs {
		if r.Role != "steward" || !r.CreatedAt.After(t) {
			continue
		}
		if best == nil || r.CreatedAt.After(best.CreatedAt) {
			best = r
		}
	}
	return best
}

// writeStewardFallbackNote records that the steward stopped WITHOUT leaving a
// briefing.
//
// It exists because the alternative is silence, and silence is the failure this
// whole feature is built against: the next steward finds no note, concludes
// there was nothing in flight, and starts over. A record that says "the agent
// did not answer — reconcile the journal" is worth far more than nothing, and it
// is honest about being a pointer rather than a handover.
func writeStewardFallbackNote(sess *stewardSession, reason string) (*handoff.Record, error) {
	rec, err := stewardFallbackRecord(sess, reason)
	if err != nil {
		return nil, err
	}
	if _, err := handoff.Save(handoff.DefaultDir(), rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// stewardFallbackRecord builds the record without storing it. Split out because
// the WORDS are the contract here — a fallback note that reads like a briefing
// is worse than no note at all — and they must be assertable without touching
// the operator's real handoff store.
func stewardFallbackRecord(sess *stewardSession, reason string) (*handoff.Record, error) {
	now := time.Now().UTC()
	cwd := sess.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	rec := &handoff.Record{
		SchemaVersion: handoff.SchemaVersion,
		ID:            handoff.NewID(now, cwd),
		CreatedAt:     now,
		From:          steward.Self(),
		Role:          "steward",
		Project:       handoff.Project{Name: "host", Primary: cwd, Roots: []string{cwd}, Inferred: []string{"steward-session"}},
		Continuity: fmt.Sprintf(
			"AUTOMATIC RECORD — NOT A BRIEFING. The steward session (%s, %s) stopped: %s. "+
				"The agent did not produce a handoff note before it went away, so nothing here describes what it was "+
				"actually doing. Treat this as a marker that a tenure ended untidily, not as continuity.",
			sess.Agent, sess.Binding, reason),
		NextAction: "Run `bashy steward reconcile` first — it reports what the journal can and cannot establish. " +
			"Then `bashy steward log --degraded` for claims nobody checked, and `bashy board` for work in flight. " +
			"Do not assume the host was idle.",
		Blockers: []string{"no briefing from the previous steward: this record was written mechanically at stop"},
	}
	return rec, nil
}

func stewardFirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}
