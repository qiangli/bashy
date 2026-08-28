package agentos

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/meet"
)

// wireMessageBoard connects `bashy mb` to the fleet catalog.
//
// pkg/bus is the transport and the roster is policy, so every question the
// board asks about the fleet is a hook the host fills in. All four are wired
// HERE, in one function, for a reason that is the whole point of the file:
//
// A seam nobody connects is indistinguishable from a feature nobody built —
// except that it looks finished. This codebase has produced that shape five
// times in a single day (an ExecHandler never registered, a struct field with
// no producer, a bus with no subscriptions, a routing hint on a path nothing
// reached, and DetectHarness's own predecessor). Every one of them passed its
// unit tests, because a test that calls the mechanism directly proves the
// mechanism works and says nothing about whether anything calls it.
//
// So the wiring is a named function with a test that asserts each hook is
// non-nil after it runs. That test fails if someone adds a fifth hook and
// forgets it here — which is the only failure mode this class of bug has.
func wireMessageBoard() {
	bus.FleetNames = fleetAgentNames
	bus.FleetSelect = fleetSelectAudience
	bus.FleetResolveName = fleetResolveAgentName

	// WHO you are, not merely who you can address.
	//
	// Without this, an agent in a third-party TUI — no BASHY_PRINCIPAL, and it
	// inherits the operator's environment — falls through to $USER, so its
	// posts are SIGNED with the operator's name, its cursor advances under that
	// name, and any claim it takes is recorded against it. Measured on this
	// host 2026-08-03: six of eight posts on a live board read `from: qiangli`,
	// spanning the operator and two different agents.
	//
	// fleet.DetectTool is the registry-driven marker table (`bashy tools add`
	// extends it), so the board and the rest of bashy agree on what counts as
	// an agent rather than keeping a second copy that can drift.
	bus.DetectHarness = fleet.DetectTool
	// One receive surface, one turn-boundary hook. The callback is a read-through
	// view over MB/Meet/role stores; pkg/bus remains the owner of pending delivery
	// and no additional spool is introduced.
	bus.PrepareTurnInbox = unifiedTurnPreamble
}

// notifyMeetInvitation is the write half of meet's message-board seam. The
// durable board append happens first; live steering is an immediacy upgrade,
// never a substitute for the copy the recipient can read later.
func notifyMeetInvitation(agent string, inv meet.Invitation) (bool, string, error) {
	body := "meeting invitation"
	if topic := strings.TrimSpace(inv.Topic); topic != "" {
		body += ": " + topic
	}
	body += "\n" + inv.Join

	if err := bus.PostMessage(bus.Post{
		From:  "meet",
		To:    agent,
		Topic: "meet",
		Body:  body,
	}); err != nil {
		return false, "", err
	}

	delivery := bus.SteerLive(agent, "[meet] "+body+"\n(also on the board — `bashy mb`)")
	if delivery.Steered {
		return true, "delivered live and posted to mb", nil
	}
	if delivery.Reason != "" {
		return true, "posted to mb; live delivery unavailable: " + delivery.Reason, nil
	}
	return true, "posted to mb", nil
}

// postMeetMessageBoardPost is the WRITE half of the seam — a board's own
// announcements (the pointer back when it seeds from a thread, a group invite,
// the outcome when it closes), as distinct from notifyMeetInvitation, which
// delivers a per-agent invitation.
//
// A nil audience is a broadcast. A non-nil one carries meet's OpenInvite, which
// is projected onto bus.Audience field-for-field: the selector vocabulary is
// mb's, and Any is simply the audience with no constraint set, so an open board
// can never admit somebody `mb send` would not have reached.
func postMeetMessageBoardPost(post meet.MBPost, inv *meet.OpenInvite) (int64, error) {
	from := strings.TrimSpace(post.From)
	if from == "" {
		from = "meet"
	}
	p := bus.Post{From: from, Topic: post.Topic, Body: post.Body}
	if inv != nil {
		p.Audience = &bus.Audience{
			Band: inv.Band, Tool: inv.Tool, Provider: inv.Provider,
			Family: inv.Family, Version: inv.Version,
		}
	}
	return bus.PostMessageSeq(p)
}

// fetchMeetMessageBoardPosts is the read half of the seam. Meet owns the
// requested ordering contract, so translate bus posts by sequence rather than
// leaking bus's storage order into the room seed.
func fetchMeetMessageBoardPosts(seqs []int64) ([]meet.MBPost, error) {
	posts, err := bus.Posts()
	if err != nil {
		return nil, err
	}
	bySeq := make(map[int64]bus.Post, len(posts))
	for _, post := range posts {
		bySeq[post.Seq] = post
	}

	out := make([]meet.MBPost, 0, len(seqs))
	for _, seq := range seqs {
		post, ok := bySeq[seq]
		if !ok {
			return nil, fmt.Errorf("mb post #%d not found", seq)
		}
		out = append(out, meet.MBPost{
			Seq: seq, From: post.From, Topic: post.Topic, Body: post.Body,
		})
	}
	return out, nil
}
