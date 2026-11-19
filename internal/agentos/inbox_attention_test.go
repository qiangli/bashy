package agentos

import (
	"bytes"
	"encoding/json"
	"github.com/qiangli/yoke/pkg/ask"
	"github.com/qiangli/yoke/pkg/bus"
	"testing"
)

func TestAttentionKeepsPendingAcrossPreferencesAndValidatesReview(t *testing.T) {
	n := attentionNotice{Schema: "bashy-attention-notice-v1", Initiative: "sprint:666", Kind: "review", Summary: "ready", Artifacts: []string{"git:abc:change.diff"}, Consequences: "Replaces the current panel", Decision: "Accept this milestone?", Revision: "abc"}
	body, _ := json.Marshal(n)
	mail := []mailboxItem{{ID: "mb:1", Body: string(body), Read: true}, {ID: "mb:1", Body: string(body)}, {ID: "mb:2", Status: "conflict", Project: "sprint:666", Body: "owner unresolved"}, {ID: "mb:3", Status: "review", Body: "done"}, {ID: "mb:4", Status: "review", Acknowledged: true}}
	for _, p := range []string{"observe", "answer-questions", "review-milestones", "direct-work"} {
		gs := composeAttention(mail, nil, map[string]string{"sprint:666": p})
		if len(gs) != 3 || gs[0].Urgency != "urgent" {
			t.Fatalf("%s: %#v", p, gs)
		}
		count := 0
		for _, g := range gs {
			for _, i := range g.Items {
				count++
				if i.ID == "mb:1" && (!i.Ready || i.Revision != "abc") {
					t.Fatal(i)
				}
				if i.ID == "mb:3" && (i.Ready || len(i.Missing) != 4) {
					t.Fatal(i)
				}
			}
		}
		if count != 3 {
			t.Fatalf("pending lost or duplicated: %d", count)
		}
	}
}

func TestAttentionCommandPreservesMailboxAndApproval(t *testing.T) {
	spec := isolateMailbox(t)
	t.Setenv("BASHY_ASK_DIR", t.TempDir())
	if err := bus.PostMessage(bus.Post{From: "worker", To: spec.Address, Body: "question"}); err != nil {
		t.Fatal(err)
	}
	r, err := ask.Raise(ask.ApprovalSpec{Source: ask.SourceConfirm, Argv: []string{"deploy", "candidate-a"}, Sprint: "sprint:666", Effects: []string{"write"}})
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := snapshotMailbox(spec)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		cmd := newAttentionCmd(true)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--json", "--initiative", "sprint:666", "--participation", "direct-work"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out.Bytes(), []byte(r.ID)) {
			t.Fatal(out.String())
		}
	}
	after, state, err := snapshotMailbox(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) || after[0].Acknowledged || after[0].Read || state.Participation["sprint:666"] != "direct-work" {
		t.Fatal(after, state)
	}
	if _, ok := ask.LoadDecision(r.ID); ok {
		t.Fatal("preference approved operation")
	}
	if _, err := ask.Decide(r.ID, ask.Decision{Verdict: ask.VerdictAllow, By: "operator", Via: ask.ViaHuman}, ask.DecideOpts{}); err != nil {
		t.Fatal(err)
	}
	changed, err := ask.Raise(ask.ApprovalSpec{Source: ask.SourceConfirm, Argv: []string{"deploy", "candidate-b"}, Sprint: "sprint:666"})
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID == r.ID || changed.Approval.Token == r.Approval.Token {
		t.Fatal("changed operation reused approval")
	}
	pending, err := ask.InspectPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != changed.ID {
		t.Fatal("changed operation did not require fresh approval", pending)
	}
	gs := composeAttention(nil, []ask.Request{r, changed}, nil)
	if len(gs) != 1 || len(gs[0].Items) != 2 {
		t.Fatal(gs)
	}
}
