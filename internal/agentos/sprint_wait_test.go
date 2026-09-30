package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/weave"
)

func sprintWaitSnapshotWithColumn(id int64, column string, thread []weaveCommentView) *sprintWaitSnapshot {
	return &sprintWaitSnapshot{Column: column, Thread: thread}
}

func TestSprintWaitStoryAccepted(t *testing.T) {
	sprintWatchIsolate(t)
	prev := sprintWaitSnapshotWithColumn(1, "doing", []weaveCommentView{{Kind: "note", Body: "initial"}})
	cur := sprintWaitSnapshotWithColumn(1, "doing", []weaveCommentView{{Kind: "note", Body: "initial"}, {Kind: "decision", Body: "alice accepted story abcd for sprint"}})
	calls := 0
	rt := sprintWaitRuntime{
		now:       time.Now,
		pollEvery: 5 * time.Millisecond,
		read: func(int64) (*sprintWaitSnapshot, error) {
			calls++
			if calls == 1 {
				return prev, nil
			}
			return cur, nil
		},
		sleep: func(ctx context.Context, d time.Duration) error {
			time.Sleep(1 * time.Millisecond)
			return nil
		},
	}
	var out bytes.Buffer
	err := runSprintWait(context.Background(), &out, 1, 0, false, rt)
	if err != nil {
		t.Fatalf("wait error: %v", err)
	}
	if !strings.Contains(out.String(), "story_accepted") {
		t.Fatalf("want story_accepted, got %q", out.String())
	}
}

func TestSprintWaitStoryRejected(t *testing.T) {
	prev := &sprintWaitSnapshot{Column: "doing", Thread: []weaveCommentView{{Kind: "note", Body: "x"}}}
	cur := &sprintWaitSnapshot{Column: "doing", Thread: []weaveCommentView{{Kind: "note", Body: "x"}, {Kind: "fail", Body: "bob failed story efgh"}}}
	calls := 0
	rt := sprintWaitRuntime{
		now:       time.Now,
		pollEvery: 5 * time.Millisecond,
		read: func(int64) (*sprintWaitSnapshot, error) {
			calls++
			if calls == 1 {
				return prev, nil
			}
			return cur, nil
		},
		sleep: func(ctx context.Context, d time.Duration) error { return nil },
	}
	var out bytes.Buffer
	if err := runSprintWait(context.Background(), &out, 2, 0, false, rt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "story_rejected") {
		t.Fatalf("want story_rejected got %q", out.String())
	}
}

func TestSprintWaitStorySubmitted(t *testing.T) {
	prev := &sprintWaitSnapshot{Column: "doing", Thread: []weaveCommentView{}}
	cur := &sprintWaitSnapshot{Column: "doing", Thread: []weaveCommentView{{Kind: "decision", Body: "carol submitted story 1234 for merge/closure"}}}
	calls := 0
	rt := sprintWaitRuntime{
		now:       time.Now,
		pollEvery: 5 * time.Millisecond,
		read: func(int64) (*sprintWaitSnapshot, error) {
			calls++
			if calls == 1 {
				return prev, nil
			}
			return cur, nil
		},
		sleep: func(ctx context.Context, d time.Duration) error { return nil },
	}
	var out bytes.Buffer
	if err := runSprintWait(context.Background(), &out, 3, 0, false, rt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "story_submitted") {
		t.Fatalf("want story_submitted got %q", out.String())
	}
}

func TestSprintWaitStageChange(t *testing.T) {
	prev := &sprintWaitSnapshot{Column: "doing", Thread: []weaveCommentView{}}
	cur := &sprintWaitSnapshot{Column: "done", Thread: []weaveCommentView{}}
	calls := 0
	rt := sprintWaitRuntime{
		now:       time.Now,
		pollEvery: 5 * time.Millisecond,
		read: func(int64) (*sprintWaitSnapshot, error) {
			calls++
			if calls == 1 {
				return prev, nil
			}
			return cur, nil
		},
		sleep: func(ctx context.Context, d time.Duration) error { return nil },
	}
	var out bytes.Buffer
	if err := runSprintWait(context.Background(), &out, 4, 0, false, rt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "stage") {
		t.Fatalf("want stage got %q", out.String())
	}
	if !strings.Contains(out.String(), "done") {
		t.Fatalf("want done in %q", out.String())
	}
}

func TestSprintWaitThreadBlocker(t *testing.T) {
	prev := &sprintWaitSnapshot{Column: "doing", Thread: []weaveCommentView{}}
	cur := &sprintWaitSnapshot{Column: "doing", Thread: []weaveCommentView{{Kind: "blocker", Body: "need decision on API"}}}
	calls := 0
	rt := sprintWaitRuntime{
		now:       time.Now,
		pollEvery: 5 * time.Millisecond,
		read: func(int64) (*sprintWaitSnapshot, error) {
			calls++
			if calls == 1 {
				return prev, nil
			}
			return cur, nil
		},
		sleep: func(ctx context.Context, d time.Duration) error { return nil },
	}
	var out bytes.Buffer
	if err := runSprintWait(context.Background(), &out, 5, 0, false, rt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "blocker") {
		t.Fatalf("want blocker got %q", out.String())
	}
}

func TestSprintWaitThreadErrorKind(t *testing.T) {
	prev := &sprintWaitSnapshot{Column: "doing", Thread: []weaveCommentView{}}
	cur := &sprintWaitSnapshot{Column: "doing", Thread: []weaveCommentView{{Kind: "error", Body: "gate failed"}}}
	calls := 0
	rt := sprintWaitRuntime{
		now:       time.Now,
		pollEvery: 5 * time.Millisecond,
		read: func(int64) (*sprintWaitSnapshot, error) {
			calls++
			if calls == 1 {
				return prev, nil
			}
			return cur, nil
		},
		sleep: func(ctx context.Context, d time.Duration) error { return nil },
	}
	var out bytes.Buffer
	if err := runSprintWait(context.Background(), &out, 6, 0, false, rt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "blocker") {
		t.Fatalf("want blocker for error kind got %q", out.String())
	}
}

func TestSprintWaitConductorIdle(t *testing.T) {
	prev := &sprintWaitSnapshot{Column: "doing"}
	cur := &sprintWaitSnapshot{Column: "doing", Idle: true, IdleInfo: "log stale"}
	calls := 0
	rt := sprintWaitRuntime{
		now:       time.Now,
		pollEvery: 5 * time.Millisecond,
		read: func(int64) (*sprintWaitSnapshot, error) {
			calls++
			if calls == 1 {
				return prev, nil
			}
			return cur, nil
		},
		sleep: func(ctx context.Context, d time.Duration) error { return nil },
	}
	var out bytes.Buffer
	if err := runSprintWait(context.Background(), &out, 7, 0, false, rt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "conductor_idle") {
		t.Fatalf("want conductor_idle got %q", out.String())
	}
}

func TestSprintWaitDiskLow(t *testing.T) {
	prev := &sprintWaitSnapshot{Column: "doing"}
	cur := &sprintWaitSnapshot{Column: "doing", DiskLow: true, DiskInfo: "disk low"}
	calls := 0
	rt := sprintWaitRuntime{
		now:       time.Now,
		pollEvery: 5 * time.Millisecond,
		read: func(int64) (*sprintWaitSnapshot, error) {
			calls++
			if calls == 1 {
				return prev, nil
			}
			return cur, nil
		},
		sleep: func(ctx context.Context, d time.Duration) error { return nil },
	}
	var out bytes.Buffer
	if err := runSprintWait(context.Background(), &out, 8, 0, false, rt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "disk_low") {
		t.Fatalf("want disk_low got %q", out.String())
	}
}

func TestSprintWaitTimeout(t *testing.T) {
	snap := &sprintWaitSnapshot{Column: "doing"}
	rt := sprintWaitRuntime{
		now:       time.Now,
		pollEvery: 5 * time.Millisecond,
		read: func(int64) (*sprintWaitSnapshot, error) {
			return snap, nil
		},
		sleep: func(ctx context.Context, d time.Duration) error {
			// simulate timeout expiration by sleeping past deadline
			time.Sleep(2 * time.Millisecond)
			return nil
		},
	}
	var out bytes.Buffer
	err := runSprintWait(context.Background(), &out, 9, 15*time.Millisecond, false, rt)
	if err == nil {
		t.Fatal("want timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("want timeout in error, got %v", err)
	}
}

func TestSprintWaitJSONOutput(t *testing.T) {
	prev := &sprintWaitSnapshot{Column: "doing"}
	cur := &sprintWaitSnapshot{Column: "doing", Thread: []weaveCommentView{{Kind: "blocker", Body: "blocked"}}}
	calls := 0
	rt := sprintWaitRuntime{
		now:       time.Now,
		pollEvery: 5 * time.Millisecond,
		read: func(int64) (*sprintWaitSnapshot, error) {
			calls++
			if calls == 1 {
				return prev, nil
			}
			return cur, nil
		},
		sleep: func(ctx context.Context, d time.Duration) error { return nil },
	}
	var out bytes.Buffer
	if err := runSprintWait(context.Background(), &out, 10, 0, true, rt); err != nil {
		t.Fatal(err)
	}
	var ev sprintWaitEvent
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &ev); err != nil {
		t.Fatalf("json unmarshal: %v out=%q", err, out.String())
	}
	if ev.SchemaVersion != sprintWaitSchema {
		t.Fatalf("schema %q want %q", ev.SchemaVersion, sprintWaitSchema)
	}
	if ev.Type == "" {
		t.Fatal("empty type in json")
	}
}

func TestSprintWaitReadsStructuredData(t *testing.T) {
	// Verify that the default reader hits structured queue.json, not text.
	// We isolate and write a sprint card directly, then wait for stage change.
	sprintWatchIsolate(t)
	dir := func() string {
		d, _ := weave.SprintStoreDir()
		return d
	}()
	// create initial sprint
	initial := sprintWaitQueue{Stories: []*sprintWaitStory{{ID: 99, Column: "doing", Thread: []weaveCommentView{}}}}
	b, _ := json.Marshal(initial)
	if err := writeFile(dir+"/queue.json", b); err != nil {
		t.Fatal(err)
	}
	// schedule update after short delay
	go func() {
		time.Sleep(20 * time.Millisecond)
		updated := sprintWaitQueue{Stories: []*sprintWaitStory{{ID: 99, Column: "done", Thread: []weaveCommentView{}}}}
		bb, _ := json.Marshal(updated)
		_ = writeFile(dir+"/queue.json", bb)
	}()
	rt := defaultSprintWaitRuntime()
	rt.pollEvery = 5 * time.Millisecond
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runSprintWait(ctx, &out, 99, 0, false, rt); err != nil {
		t.Fatalf("structured read failed: %v", err)
	}
	if !strings.Contains(out.String(), "stage") {
		t.Fatalf("want stage from structured data, got %q", out.String())
	}
}

func writeFile(path string, data []byte) error {
	dir := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			dir = path[:i]
			break
		}
	}
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
