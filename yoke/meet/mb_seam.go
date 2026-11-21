// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

// meet's own copy of the message-board seam (write/read halves), duplicated
// from internal/agentos/messageboard.go on purpose (see steward_select.go).

import (
	"fmt"
	"strings"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/meet"
)

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
