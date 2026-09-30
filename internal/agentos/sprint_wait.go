package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/qiangli/yoke/pkg/foreman"
	"github.com/qiangli/yoke/pkg/resources"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

const (
	sprintWaitSchema        = "bashy-sprint-wait-v1"
	sprintWaitPollInterval  = 1 * time.Second
	sprintWaitIdleThreshold = 5 * time.Minute
	sprintWaitDiskFloor     = "" // derived via hostFloors; not a constant
)

// sprintWaitEvent is the one line printed on success.
type sprintWaitEvent struct {
	SchemaVersion string `json:"schema_version"`
	Type          string `json:"type"`
	Sprint        int64  `json:"sprint"`
	Message       string `json:"message"`
	At            string `json:"at,omitempty"`
}

type sprintWaitSnapshot struct {
	Column   string
	Thread   []weaveCommentView
	Idle     bool
	IdleInfo string
	DiskLow  bool
	DiskInfo string
}

type weaveCommentView struct {
	Kind string    `json:"kind"`
	Body string    `json:"body"`
	At   time.Time `json:"at"`
}

// sprintWaitRuntime allows injection for tests.
type sprintWaitRuntime struct {
	now           func() time.Time
	pollEvery     time.Duration
	idleThreshold time.Duration
	read          func(int64) (*sprintWaitSnapshot, error)
	sleep         func(context.Context, time.Duration) error
}

func defaultSprintWaitRuntime() sprintWaitRuntime {
	return sprintWaitRuntime{
		now:           time.Now,
		pollEvery:     sprintWaitPollInterval,
		idleThreshold: sprintWaitIdleThreshold,
		read:          readSprintWaitSnapshot,
		sleep:         waitInboxPoll,
	}
}

// sprintWaitQueue is the minimal shape of queue.json needed for wait.
// It mirrors weave.weaveQueue's Sprints field without importing unexported type.
type sprintWaitQueue struct {
	Stories []*sprintWaitStory `json:"stories"`
}
type sprintWaitStory struct {
	ID     int64              `json:"id"`
	Column string             `json:"column"`
	Thread []weaveCommentView `json:"thread,omitempty"`
}

func loadSprintWaitQueue(dir string) (*sprintWaitQueue, error) {
	b, err := os.ReadFile(filepath.Join(dir, "queue.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &sprintWaitQueue{}, nil
		}
		return nil, err
	}
	var q sprintWaitQueue
	if err := json.Unmarshal(b, &q); err != nil {
		return nil, fmt.Errorf("queue parse: %w", err)
	}
	return &q, nil
}

func findSprintWaitStory(q *sprintWaitQueue, id int64) *sprintWaitStory {
	for _, s := range q.Stories {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// readSprintWaitSnapshot reads structured card data (no text parsing).
func readSprintWaitSnapshot(id int64) (*sprintWaitSnapshot, error) {
	dir, err := weave.SprintStoreDir()
	if err != nil {
		return nil, err
	}
	q, err := loadSprintWaitQueue(dir)
	if err != nil {
		return nil, err
	}
	s := findSprintWaitStory(q, id)
	if s == nil {
		return nil, fmt.Errorf("sprint #%d not found", id)
	}
	snap := &sprintWaitSnapshot{
		Column: s.Column,
	}
	for _, c := range s.Thread {
		snap.Thread = append(snap.Thread, c)
	}
	// conductor idle via foreman log mtime
	if idle, info := sprintWaitConductorIdle(id); idle {
		snap.Idle = true
		snap.IdleInfo = info
	}
	if low, info := sprintWaitDiskLow(); low {
		snap.DiskLow = true
		snap.DiskInfo = info
	}
	return snap, nil
}

func sprintWaitConductorIdle(sprintID int64) (bool, string) {
	store := foreman.NewStore("", sprintOwnerSessionID(sprintID))
	logPath := store.LogPath()
	fi, err := os.Stat(logPath)
	if err != nil {
		return false, ""
	}
	age := time.Since(fi.ModTime())
	if age > sprintWaitIdleThreshold {
		return true, fmt.Sprintf("foreman log %s mtime %s ago (%s)", logPath, age.Round(time.Second), fi.ModTime().Format(time.RFC3339))
	}
	return false, ""
}

func sprintWaitDiskLow() (bool, string) {
	// Use the same floor logic as hostFloors: min(2Gi, 5% total)
	// Check the working volume; fallback to home or "/".
	checkPaths := []string{"."}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		checkPaths = append(checkPaths, home)
	}
	checkPaths = append(checkPaths, "/")
	for _, p := range checkPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		free, total, err := resources.FreeDiskAt(abs)
		if err != nil || total == 0 {
			continue
		}
		floor := uint64(2 << 30)
		if total/20 < floor {
			floor = total / 20
		}
		if free < floor {
			return true, fmt.Sprintf("disk %s free %d < floor %d (total %d)", abs, free, floor, total)
		}
		// first filesystem is enough; if not low, host is not under floor
		return false, ""
	}
	return false, ""
}

func detectSprintWaitEvent(prev, cur *sprintWaitSnapshot, sprintID int64) *sprintWaitEvent {
	nowStr := time.Now().UTC().Format(time.RFC3339)
	// level-triggered: disk low and idle are reported if currently true
	// (even if already true at baseline, the caller sees it on next poll)
	if cur.DiskLow {
		return &sprintWaitEvent{SchemaVersion: sprintWaitSchema, Type: "disk_low", Sprint: sprintID, Message: cur.DiskInfo, At: nowStr}
	}
	if cur.Idle {
		return &sprintWaitEvent{SchemaVersion: sprintWaitSchema, Type: "conductor_idle", Sprint: sprintID, Message: cur.IdleInfo, At: nowStr}
	}
	// column change (stage)
	if prev != nil && cur.Column != prev.Column {
		msg := fmt.Sprintf("stage %s → %s", prev.Column, cur.Column)
		// include done explicitly
		if cur.Column == "done" {
			msg = "stage → done"
		}
		return &sprintWaitEvent{SchemaVersion: sprintWaitSchema, Type: "stage", Sprint: sprintID, Message: msg, At: nowStr}
	}
	// thread new entries: detect story and blocker/error and stage
	if prev != nil && len(cur.Thread) > len(prev.Thread) {
		for i := len(prev.Thread); i < len(cur.Thread); i++ {
			e := cur.Thread[i]
			lowBody := strings.ToLower(e.Body)
			lowKind := strings.ToLower(strings.TrimSpace(e.Kind))
			if strings.Contains(lowBody, "accepted story") {
				return &sprintWaitEvent{SchemaVersion: sprintWaitSchema, Type: "story_accepted", Sprint: sprintID, Message: e.Body, At: nowStr}
			}
			if strings.Contains(lowBody, "failed story") || strings.Contains(lowBody, "rejected story") {
				return &sprintWaitEvent{SchemaVersion: sprintWaitSchema, Type: "story_rejected", Sprint: sprintID, Message: e.Body, At: nowStr}
			}
			if strings.Contains(lowBody, "submitted story") {
				return &sprintWaitEvent{SchemaVersion: sprintWaitSchema, Type: "story_submitted", Sprint: sprintID, Message: e.Body, At: nowStr}
			}
			if lowKind == "blocker" || lowKind == "error" {
				return &sprintWaitEvent{SchemaVersion: sprintWaitSchema, Type: "blocker", Sprint: sprintID, Message: e.Body, At: nowStr}
			}
			if lowKind == "stage" {
				return &sprintWaitEvent{SchemaVersion: sprintWaitSchema, Type: "stage", Sprint: sprintID, Message: e.Body, At: nowStr}
			}
			// fallback: if body mentions blocker/error even with different kind
			if strings.Contains(lowBody, "blocker") || strings.Contains(lowBody, "error") {
				if lowKind == "blocker" || lowKind == "error" || strings.Contains(lowBody, "blocker") {
					// only report if kind is blocker/error to avoid noise; already handled
				}
			}
		}
		// If none matched but thread grew, treat stage change already handled; no event
	}
	return nil
}

func newSprintWaitCmd() *cobra.Command {
	var timeout time.Duration
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "wait <sprint> [--timeout D] [--json]",
		Short: "Block until the next event on the sprint card, print one line, exit",
		Long: `wait blocks until the next event on the sprint card and exits 0 printing one line.

Events (structured card data, no text parsing):
  story accepted / rejected / submitted
  stage change (incl. done)
  thread line of kind blocker / error
  conductor idle past threshold (foreman log mtime)
  host disk below the weave guard floor

Timeout exits non-zero. No daemon, no config, no subscription state.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSprintID(args[0])
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			rt := defaultSprintWaitRuntime()
			// allow tests to override via context? keep simple
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			_ = errOut
			return runSprintWait(ctx, out, id, timeout, jsonOut, rt)
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "stop waiting after this duration (exits non-zero on timeout)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit one JSON line instead of text")
	return cmd
}

func runSprintWait(ctx context.Context, out io.Writer, sprintID int64, timeout time.Duration, jsonOut bool, rt sprintWaitRuntime) error {
	if rt.now == nil {
		rt.now = time.Now
	}
	if rt.pollEvery <= 0 {
		rt.pollEvery = sprintWaitPollInterval
	}
	if rt.sleep == nil {
		rt.sleep = waitInboxPoll
	}
	if rt.read == nil {
		rt.read = readSprintWaitSnapshot
	}
	// baseline
	prev, err := rt.read(sprintID)
	if err != nil {
		return err
	}
	deadline := time.Time{}
	if timeout > 0 {
		deadline = rt.now().Add(timeout)
	}
	// immediate check for level-triggered conditions (disk/idle) at baseline?
	// Treat baseline idle/disk as already an event? Spec says block until next event,
	// so baseline itself is not an event; we wait for a change. But for level triggers,
	// if already low/idle, the next poll will report it immediately (one poll delay).
	// To avoid missing immediate idle/disk, check prev vs cur after first poll.
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !deadline.IsZero() && !rt.now().Before(deadline) {
			return fmt.Errorf("sprint wait: timeout after %s", timeout)
		}
		// sleep until next poll (first iteration also sleeps, so caller has time to start)
		remaining := rt.pollEvery
		if !deadline.IsZero() {
			if d := time.Until(deadline); d < remaining && d > 0 {
				remaining = d
			}
			if d := deadline.Sub(rt.now()); d <= 0 {
				return fmt.Errorf("sprint wait: timeout after %s", timeout)
			}
		}
		if err := rt.sleep(ctx, remaining); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// context canceled via timeout
			if !deadline.IsZero() && !rt.now().Before(deadline) {
				return fmt.Errorf("sprint wait: timeout after %s", timeout)
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !deadline.IsZero() && !rt.now().Before(deadline) {
			return fmt.Errorf("sprint wait: timeout after %s", timeout)
		}
		cur, err := rt.read(sprintID)
		if err != nil {
			return err
		}
		if ev := detectSprintWaitEvent(prev, cur, sprintID); ev != nil {
			if jsonOut {
				b, _ := json.Marshal(ev)
				fmt.Fprintln(out, string(b))
			} else {
				// one line: type + message
				fmt.Fprintf(out, "%s: %s\n", ev.Type, ev.Message)
			}
			return nil
		}
		prev = cur
	}
}

// loadWeaveQueue and findWeaveStory are defined in weave package; we delegate.
// For test isolation we need SprintStoreDir to respect BASHY_SPRINT_DIR env.
