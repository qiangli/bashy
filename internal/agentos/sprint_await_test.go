package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
)

func TestSprintAwaitEvents(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	link := agentsSprintRun{Repo: "demo", ID: 7, Story: "todo:abcdef123456"}
	typed := weaveCommentView{Kind: "submit", StoryID: "abcdef123456", EvidenceRef: "evidence:123", Author: "worker", Body: "ready", At: now}
	legacy := weaveCommentView{Kind: "decision", Body: "worker submitted story abcdef for closure"}
	submitted, _ := sprintAwaitRunEvent(link, "demo-123", agentsQueueItem{ID: 7, State: "submitted", FinishedAt: now}, now)
	failed, _ := sprintAwaitRunEvent(link, "demo-123", agentsQueueItem{ID: 7, State: "failed", FinishedAt: now}, now)
	working := agentsQueueItem{ID: 7, State: "working", StartedAt: now.Add(-time.Hour)}
	if _, ok := sprintAwaitRunEvent(link, "demo-123", working, now.Add(-time.Second)); ok {
		t.Fatal("run stalled before the idle threshold")
	}
	stalled, _ := sprintAwaitRunEvent(link, "demo-123", working, now)
	reply := sprintAwaitCandidate{key: "mb/1", at: now, event: sprintWaitEvent{
		Type: "manager_reply", SuggestedVerb: "reply", From: "worker", Message: "here is the answer",
	}}

	for _, tc := range []struct {
		name                              string
		current                           sprintWaitSnapshot
		kind, verb, story, evidence, from string
		run                               int64
	}{
		{"submit", sprintWaitSnapshot{Thread: []weaveCommentView{legacy, typed}}, "story_submitted", "accept", typed.StoryID, typed.EvidenceRef, typed.Author, 0},
		{"run submitted", sprintWaitSnapshot{Actionable: []sprintAwaitCandidate{submitted}}, "run_submitted", "pull", typed.StoryID, "", "", 7},
		{"run failed", sprintWaitSnapshot{Actionable: []sprintAwaitCandidate{failed}}, "run_failed", "reassign", typed.StoryID, "", "", 7},
		{"run stalled", sprintWaitSnapshot{Actionable: []sprintAwaitCandidate{stalled}}, "run_stalled", "reassign", typed.StoryID, "", "", 7},
		{"reply", sprintWaitSnapshot{Actionable: []sprintAwaitCandidate{reply}}, "manager_reply", "reply", "", "", "worker", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			var polls int
			snapshots := []*sprintWaitSnapshot{{}, {}, &tc.current}
			rt := sprintWaitRuntime{
				read: func(int64) (*sprintWaitSnapshot, error) {
					s := snapshots[polls]
					polls++
					return s, nil
				},
				sleep: func(context.Context, time.Duration) error { return nil },
			}
			if err := runSprintEvent(context.Background(), &out, 42, 0, true, rt, detectSprintAwaitEvent); err != nil {
				t.Fatal(err)
			}
			var got sprintWaitEvent
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if polls != 3 || got.SchemaVersion != sprintAwaitSchema || got.Sprint != 42 ||
				got.Type != tc.kind || got.SuggestedVerb != tc.verb || got.StoryID != tc.story ||
				got.EvidenceRef != tc.evidence || got.RunID != tc.run || got.From != tc.from ||
				got.At != now.Format(time.RFC3339Nano) {
				t.Fatalf("after %d reads got %+v", polls, got)
			}
		})
	}
}

func TestSprintAwaitBaselineAndTimeout(t *testing.T) {
	old := weaveCommentView{Kind: "submit", StoryID: "old"}
	s := &sprintWaitSnapshot{Thread: []weaveCommentView{old}, Actionable: []sprintAwaitCandidate{
		{key: "mb/1", event: sprintWaitEvent{Type: "manager_reply"}},
	}}
	next := *s
	next.Thread = append(next.Thread, weaveCommentView{Kind: "decision", StoryID: "x", Body: "submitted story x"},
		weaveCommentView{Kind: "submit", Body: "missing story id"})
	now := time.Now()
	reads := 0
	rt := sprintWaitRuntime{
		now: func() time.Time { return now },
		read: func(int64) (*sprintWaitSnapshot, error) {
			reads++
			if reads == 1 {
				return s, nil
			}
			return &next, nil
		},
		sleep: func(_ context.Context, d time.Duration) error { now = now.Add(d); return nil },
	}
	var out bytes.Buffer
	err := runSprintEvent(context.Background(), &out, 42, 2*time.Second, false, rt, detectSprintAwaitEvent)
	if !errors.Is(err, context.DeadlineExceeded) || out.Len() != 0 || reads != 2 {
		t.Fatalf("got %v, output %q, reads %d", err, out.String(), reads)
	}
}

func TestSprintAwaitReadsLinkedRunTransition(t *testing.T) {
	stateHome := isolateSprintForemanTest(t)
	sprintDir := os.Getenv("BASHY_SPRINT_DIR")
	born := time.Now().UTC()
	const tag = "demo-123"
	card := sprintWaitStory{ID: 42, Created: born, Runs: []agentsSprintRun{
		{Repo: "demo", ID: 7, Queue: tag, Born: born, Story: "abcdef123456"},
	}}
	write := func(path string, v any) {
		t.Helper()
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(sprintDir, "queue.json"), sprintWaitQueue{Stories: []*sprintWaitStory{
		{ID: 99, Created: born.Add(-time.Hour)}, &card,
	}})
	for _, args := range [][]string{nil, {"last"}, {"42"}} {
		if id, err := selectSprintAwait(args); id != 42 || err != nil {
			t.Fatalf("select %v: got %d, %v", args, id, err)
		}
	}
	dir := filepath.Join(stateHome, ".bashy", "weave", tag)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	runPath := filepath.Join(dir, "queue.json")
	q := agentsQueue{Items: []agentsQueueItem{
		{ID: 7, State: "working", Created: born, Title: "deliver change"},
		{ID: 8, State: "submitted", Created: born, Title: "unlinked run"},
	}}
	write(runPath, q)

	rt := sprintWaitRuntime{read: readSprintAwaitSnapshot, sleep: func(context.Context, time.Duration) error {
		q.Items[0].State = "submitted"
		q.Items[0].FinishedAt = born.Add(time.Second)
		write(runPath, q)
		return nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var out bytes.Buffer
	if err := runSprintEvent(ctx, &out, 42, 0, false, rt, detectSprintAwaitEvent); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "demo run #7") || !strings.Contains(out.String(), "suggested: pull") {
		t.Fatalf("unexpected output %q", &out)
	}
	q.Items[0].Created = born.Add(time.Hour)
	write(runPath, q)
	snapshot, err := readSprintAwaitSnapshot(42)
	if err != nil || len(snapshot.Actionable) != 0 {
		t.Fatalf("recycled ID or unlinked run appeared: %+v, %v", snapshot, err)
	}
}

func TestSprintAwaitMailIsDirectedAndUnread(t *testing.T) {
	isolateSprintForemanTest(t)
	const owner = "fixture-manager"
	for _, p := range []bus.Post{
		{From: "worker", To: owner, Body: "reply to the manager"},
		{From: "worker", To: "other", Body: "unrelated"},
		{From: "worker", Body: "broadcast"},
	} {
		if err := bus.PostMessage(p); err != nil {
			t.Fatal(err)
		}
	}
	events, err := readSprintAwaitMail(owner, "")
	if err != nil || len(events) != 1 || events[0].event.Message != "reply to the manager" {
		t.Fatalf("got %+v, %v", events, err)
	}
	if seq := bus.SeenSeq(owner); seq != 0 {
		t.Fatalf("await advanced the mail cursor to %d", seq)
	}
}
