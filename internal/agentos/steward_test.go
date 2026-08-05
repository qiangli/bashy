// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/board"
	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/steward"
)

// THE COMPLETION VERB MUST NOT BE IN THE ROLE NAMESPACE.
//
// cobra adds it to whatever command is Execute()d, and `bashy steward` is a
// mounted subtree — so the generator it advertises is for a binary called
// `steward` that does not exist. It is also the one entry in an agent-facing
// help listing that is not a thing the role can DO.
func TestStewardStartStopAreMountedAndCompletionIsNot(t *testing.T) {
	cmd := stewardTestCommand(t)
	var start, stop bool
	for _, c := range cmd.Commands() {
		switch c.Name() {
		case "completion":
			t.Error("`bashy steward` still offers cobra's generated `completion` verb")
		case "start":
			start = true
		case "stop":
			stop = true
		}
	}
	if !start || !stop {
		t.Errorf("start=%v stop=%v — both must be mounted", start, stop)
	}
}

// A SUB-L3 CHOICE MUST BE WARNED ABOUT, IN WORDS THAT NAME THE FAILURE.
//
// "band too low" is an abstraction nobody acts on. The measured failure is that
// such an agent LOOPS and reports success anyway, and an operator who has been
// told that can recognise it when it starts.
func TestStewardBandFloorWarningNamesTheFailure(t *testing.T) {
	var b strings.Builder
	writeStewardSelection(&b, &stewardSelection{
		Chosen:    stewardCandidate{Name: "cheap", Binding: "t:m", Band: 2},
		BandFloor: true,
		Why:       "test",
	})
	out := b.String()
	for _, want := range []string{"WARNING", "BELOW", "LOOPS", "LOOKS attended"} {
		if !strings.Contains(out, want) {
			t.Errorf("the band-floor warning does not mention %q:\n%s", want, out)
		}
	}
}

func TestStewardBandFloorIsSilentAtOrAboveTheFloor(t *testing.T) {
	var b strings.Builder
	writeStewardSelection(&b, &stewardSelection{
		Chosen: stewardCandidate{Name: "ok", Binding: "t:m", Band: stewardBandFloor},
		Why:    "test",
	})
	if strings.Contains(b.String(), "WARNING") {
		t.Errorf("an L%d agent was warned about:\n%s", stewardBandFloor, b.String())
	}
}

// A SEAT YOU HAVE ALREADY PAID FOR BEATS A METERED KEY, and among equals the one
// with quota left wins. This is the rule the fleet has always followed by hand.
func TestStewardSelectionPrefersPaidSeatsThenHeadroom(t *testing.T) {
	metered := stewardCandidate{Name: "metered", Band: 4, Billing: fleet.BillingMetered, HeadroomKnown: true, Headroom: 1}
	flat := stewardCandidate{Name: "flat", Band: 4, Billing: fleet.BillingFlat, HeadroomKnown: true, Headroom: 0.3}
	if !lessStewardCandidate(flat, metered) {
		t.Error("a flat-rate seat with 30% quota left lost to a metered key with a full tank — " +
			"the marginal cost of the flat seat is zero and the metered one is money")
	}

	full := stewardCandidate{Name: "full", Band: 4, Billing: fleet.BillingFlat, HeadroomKnown: true, Headroom: 0.9}
	spent := stewardCandidate{Name: "spent", Band: 4, Billing: fleet.BillingFlat, HeadroomKnown: true, Headroom: 0.05}
	if !lessStewardCandidate(full, spent) {
		t.Error("a nearly-spent seat outranked a fresh one at the same billing class")
	}
}

// UNKNOWN HEADROOM IS NOT FULL HEADROOM.
//
// The tempting shortcut — "no ceiling recorded, so treat it as unlimited" — is
// the same class of bug as an absent test result reported as a pass. It must
// rank below a measured-and-fresh seat, and above a measured-and-nearly-spent
// one.
func TestStewardSelectionRanksUnknownHeadroomInTheMiddle(t *testing.T) {
	unknown := stewardCandidate{Name: "unknown", Band: 4, Billing: fleet.BillingFlat}
	fresh := stewardCandidate{Name: "fresh", Band: 4, Billing: fleet.BillingFlat, HeadroomKnown: true, Headroom: 0.9}
	spent := stewardCandidate{Name: "spent", Band: 4, Billing: fleet.BillingFlat, HeadroomKnown: true, Headroom: 0.01}

	if !lessStewardCandidate(fresh, unknown) {
		t.Error("an unmeasured model outranked a measured one with 90% left — unknown was read as unlimited")
	}
	if !lessStewardCandidate(unknown, spent) {
		t.Error("an unmeasured model lost to one measurably out of quota — unknown was read as exhausted")
	}
}

// A MEDIATOR THAT COSTS WHAT THE STEWARD COSTS HAS SAVED NOTHING.
func TestMediatorRefusesToMatchTheStewardsBand(t *testing.T) {
	m, why := resolveStewardMediator("", 4, 4)
	if m != nil {
		t.Skip("this host has no fleet catalog to resolve against")
	}
	if !strings.Contains(why, "not below") && !strings.Contains(why, "no mediator") {
		t.Errorf("the refusal did not explain itself: %q", why)
	}
}

// THE MEDIATOR MAY SUMMARISE. IT MAY NOT FILTER.
//
// This is the whole safety property of putting a cheap agent between the host's
// mail and its steward: a digest that does not account for every message is a
// filter nobody authorised, and it fails in the direction where the steward
// concludes there was nothing to hear. The count check is what makes that a
// property rather than a hope.
func TestMediatorDigestCoverageIsCounted(t *testing.T) {
	good := "1. [SEAT] [urgent] api-conductor — blocked on a merge token\n" +
		"2. [BOARD] [fyi] ci — nightly went green\n"
	digest, n := parseMediatorDigest(good)
	if n != 2 {
		t.Fatalf("covered %d of 2 messages: %q", n, digest)
	}

	// Prose around the lines is tolerated; the lines are what count.
	chatty := "Sure! Here is the digest:\n\n" + good + "\nLet me know if you need more."
	if _, n := parseMediatorDigest(chatty); n != 2 {
		t.Errorf("preamble broke the parse: covered %d of 2", n)
	}

	// A dropped message must NOT reach full coverage — this is the failure the
	// guard exists for.
	if _, n := parseMediatorDigest("1. [SEAT] [urgent] api — blocked\n"); n == 2 {
		t.Error("a digest that mentioned one of two messages was counted as complete")
	}

	// Nor may repetition fake it: six copies of "1." cover one message.
	repeated := strings.Repeat("1. [SEAT] [fyi] x — y\n", 6)
	if _, n := parseMediatorDigest(repeated); n != 1 {
		t.Errorf("repetition was counted as coverage: %d", n)
	}

	// Free prose covers nothing.
	if _, n := parseMediatorDigest("There are two messages, both routine."); n != 0 {
		t.Errorf("prose was counted as coverage: %d", n)
	}
}

// THE NUDGE IS A POINTER, NOT THE MAIL.
//
// Both stores mark on read. If the supervisor drained them and pasted the
// bodies, the inbox would show every message read by a process that is not the
// steward, while the steward's own record showed it never looked. The two
// channels are also reported separately: "4 messages" loses the distinction
// between a predecessor's unanswered seat mail and board chatter.
func TestMailNoticeIsAPointerAndSeparatesTheChannels(t *testing.T) {
	w := &stewardMailWatch{topic: "steward.host", subscriber: "someone"}
	w.seenSeat, w.seenBoard = 10, 20

	seat := []bus.Pending{{Seq: 11, Body: "SECRET SEAT BODY"}, {Seq: 12, Body: "another"}}
	board := []bus.Pending{{Seq: 21, Body: "SECRET BOARD BODY"}}
	newSeat, _ := newerThan(seat, w.seenSeat)
	newBoard, _ := newerThan(board, w.seenBoard)
	w.nSeat, w.nBoard = len(newSeat), len(newBoard)

	notice := stewardNoticeFor(w)
	if strings.Contains(notice, "SECRET") {
		t.Errorf("the notice carried message bodies:\n%s", notice)
	}
	for _, want := range []string{"2 new at the SEAT", "1 new on the MESSAGE BOARD", "bashy steward inbox", "bashy mb"} {
		if !strings.Contains(notice, want) {
			t.Errorf("the notice is missing %q:\n%s", want, notice)
		}
	}
}

// A NUDGE THAT COULD NOT BE DELIVERED MUST NOT MARK THE MAIL AS ANNOUNCED.
//
// Otherwise a busy agent that missed one turn-boundary window never hears about
// those messages at all — the delivery is lost silently, which is the exact
// shape of failure the seat inbox exists to prevent.
func TestMailWatchCommitsOnlyAfterDelivery(t *testing.T) {
	w := &stewardMailWatch{}
	items := []bus.Pending{{Seq: 7}}
	_, w.pendSeat = newerThan(items, w.seenSeat)
	if w.seenSeat != 0 {
		t.Fatal("polling advanced the cursor before the notice was delivered")
	}
	w.commit()
	if w.seenSeat != 7 {
		t.Errorf("commit did not advance the cursor: %d", w.seenSeat)
	}
}

// THE THREE HANDOFF SITUATIONS GET THREE DIFFERENT INSTRUCTIONS.
//
// "no note" and "a day-old note" call for opposite first moves, and an agent
// given one instruction for both will take the reassuring reading.
func TestBootstrapBriefBranchesOnTheNoteState(t *testing.T) {
	sess := &stewardSession{Agent: "a", Binding: "t:m", SeatHeld: true, Epoch: 3, Topic: "steward.host"}
	brief, _ := stewardBootstrapBrief(sess, stewardStartOptions{StaleAfter: time.Hour})

	// Whatever this host's handoff store holds, the brief must always load the
	// role, name the seat, and point at the two mail verbs rather than paste mail.
	for _, want := range []string{"bashy steward skill", "bashy steward inbox", "bashy mb", "COUNT, never the contents"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the bootstrap brief is missing %q", want)
		}
	}
	// And it must always end by telling the agent how the tenure ends.
	if !strings.Contains(brief, "stand down") {
		t.Error("the brief never tells the agent it will be asked to stand down")
	}
}

// THE WRAP-UP MUST ASK FOR A COMMAND, NOT FOR PROSE.
//
// Prose in the transcript dies with the session; only a handoff record survives
// it. This is the message that has to work when the agent is mid-task and about
// to be terminated.
func TestWrapUpAsksForTheHandoffCommand(t *testing.T) {
	msg := stewardWrapUpInstruction(&stewardSession{SeatHeld: true})
	if !strings.Contains(msg, "bashy handoff --as steward") {
		t.Error("the wrap-up does not name the command that actually records a note")
	}
	if !strings.Contains(msg, "prose in this conversation dies") {
		t.Error("the wrap-up does not say why a reply is not enough")
	}
	if !strings.Contains(msg, "Do NOT release the seat") {
		t.Error("a seat-holding wrap-up must tell the agent bashy releases the seat, not it")
	}
}

// A FALLBACK NOTE MUST NEVER READ AS A HANDOVER.
//
// It exists so a successor reconciles instead of assuming an idle host — which
// only works if it is unmistakably not a briefing.
func TestFallbackNoteDoesNotImpersonateABriefing(t *testing.T) {
	sess := &stewardSession{Agent: "a", Binding: "t:m"}
	// Build the record without saving it: the assertion is about its WORDS.
	rec, err := stewardFallbackRecord(sess, "killed")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"AUTOMATIC RECORD", "NOT A BRIEFING", "did not produce a handoff note"} {
		if !strings.Contains(rec.Continuity, want) {
			t.Errorf("the fallback note is missing %q:\n%s", want, rec.Continuity)
		}
	}
	if !strings.Contains(rec.NextAction, "reconcile") {
		t.Error("the fallback note does not send the successor to `steward reconcile`")
	}
	if len(rec.Blockers) == 0 {
		t.Error("the fallback note records no blocker — the missing briefing IS one")
	}
	if rec.Role != "steward" {
		t.Errorf("the fallback note is not a seat handoff: role=%q", rec.Role)
	}
}

// stewardNoticeFor renders the notice from an already-counted watch, so the
// wording can be tested without a live bus.
func stewardNoticeFor(w *stewardMailWatch) string {
	notice, _, _ := w.render()
	return notice
}

// stewardTestCommand assembles the `bashy steward` tree the same way the
// dispatch in agentos.go does. Assembled rather than exported so the test fails
// if the two ever drift apart in SHAPE — which is the thing being asserted.
func stewardTestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	root := board.NewStewardCommand(func(io.Writer) error { return nil }, nil)
	for _, sub := range steward.NewStewardCmd().Commands() {
		root.AddCommand(sub)
	}
	root.AddCommand(newStewardStartCmd(), newStewardStopCmd())
	return root
}
