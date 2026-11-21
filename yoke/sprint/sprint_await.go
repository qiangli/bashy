package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/meet"
	"github.com/qiangli/yoke/pkg/ref"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

const (
	sprintAwaitSchema  = "bashy-sprint-await-v1"
	sprintAwaitRunIdle = time.Hour
)

type sprintAwaitCandidate struct {
	key   string
	at    time.Time
	event sprintWaitEvent
}

func newSprintAwaitCmd() *cobra.Command {
	var timeout time.Duration
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "await [sprint]",
		Short: "Block until the next actionable sprint event and suggest a verb",
		Long: `Wait for a new typed story submission, a linked weave run that submits,
fails, or stalls, or a message addressed to the sprint manager. Prints
one event and a suggested verb. An active run stalls after an hour without
recorded progress. Existing events form the baseline. Mail stays unread.

Use a sprint number, UUID, slug, or last. Omit the sprint to select the
most recently created card. Timeout and interruption exit nonzero.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeout < 0 {
				return sprintUsagef("--timeout must be nonnegative")
			}
			id, err := selectSprintAwait(args)
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
			rt := sprintWaitRuntime{read: readSprintAwaitSnapshot}
			return runSprintEvent(ctx, cmd.OutOrStdout(), id, timeout, jsonOut, rt, detectSprintAwaitEvent)
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "stop waiting after this duration")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit one JSON event")
	return cmd
}

func selectSprintAwait(args []string) (int64, error) {
	if len(args) > 0 && args[0] != "last" {
		reg := ref.NewRegistry()
		weave.RegisterRefs(reg)
		node, err := reg.Resolve("sprint:" + strings.TrimPrefix(args[0], "sprint:"))
		if err != nil {
			return 0, err
		}
		return parseSprintID(node.ID)
	}
	dir, err := weave.SprintStoreDir()
	if err != nil {
		return 0, err
	}
	q, err := loadSprintWaitQueue(dir)
	if err != nil {
		return 0, err
	}
	var latest *sprintWaitStory
	for _, s := range q.Stories {
		if s != nil && (latest == nil || s.Created.After(latest.Created) ||
			(s.Created.Equal(latest.Created) && s.ID > latest.ID)) {
			latest = s
		}
	}
	if latest == nil {
		return 0, fmt.Errorf("no sprint exists to default to")
	}
	return latest.ID, nil
}

func readSprintAwaitSnapshot(id int64) (*sprintWaitSnapshot, error) {
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
	snap := &sprintWaitSnapshot{Thread: s.Thread}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	// Queue locations follow Yoke's weave store, which is independent of BASHY_HOME.
	roots := defaultAgentWeaveRoots(home)
	for _, link := range s.Runs {
		item, queue, err := readSprintAwaitRun(roots, link)
		if err != nil {
			return nil, err
		}
		if item != nil {
			if event, ok := sprintAwaitRunEvent(link, queue, *item, time.Now()); ok {
				snap.Actionable = append(snap.Actionable, event)
			}
		}
	}
	owner := s.Owner
	if s.Lease != nil && s.Lease.Holder != "" {
		owner = s.Lease.Holder
	}
	mail, err := readSprintAwaitMail(owner, s.Contact.RefID())
	if err != nil {
		return nil, err
	}
	snap.Actionable = append(snap.Actionable, mail...)
	return snap, nil
}

func readSprintAwaitRun(roots []string, link agentsSprintRun) (*agentsQueueItem, string, error) {
	if link.Repo == "" || filepath.Base(link.Repo) != link.Repo ||
		(link.Queue != "" && (filepath.Base(link.Queue) != link.Queue || link.Queue == "." || link.Queue == "..")) {
		return nil, "", fmt.Errorf("invalid queue link for weave run #%d", link.ID)
	}
	var found *agentsQueueItem
	var queue string
	for _, root := range roots {
		tags := []string{link.Queue}
		if link.Queue == "" {
			tags = nil
			entries, err := os.ReadDir(root)
			if err != nil && !os.IsNotExist(err) {
				return nil, "", err
			}
			for _, entry := range entries {
				tag := entry.Name()
				if entry.IsDir() && strings.HasPrefix(tag, link.Repo+"-") &&
					!strings.Contains(strings.TrimPrefix(tag, link.Repo+"-"), "-") {
					tags = append(tags, tag)
				}
			}
		}
		for _, tag := range tags {
			var q agentsQueue
			if _, err := readAgentJSON(filepath.Join(root, tag, "queue.json"), &q); err != nil {
				return nil, "", err
			}
			if q.Root != "" && filepath.Base(q.Root) != link.Repo {
				continue
			}
			for i := range q.Items {
				item := &q.Items[i]
				if item.ID != link.ID || (!link.Born.IsZero() && !link.Born.Equal(item.Created)) {
					continue
				}
				// A legacy link can match more than one checkout. Only a
				// uniquely identified run can generate an actionable event.
				if found != nil {
					return nil, "", nil
				}
				found, queue = item, tag
			}
		}
	}
	return found, queue, nil
}

func sprintAwaitRunEvent(link agentsSprintRun, queue string, item agentsQueueItem, now time.Time) (sprintAwaitCandidate, bool) {
	at := item.FinishedAt
	state, verb := item.State, "reassign"
	switch state {
	case "submitted":
		verb = "pull"
	case "failed", "killed", "no-op":
		state = "failed"
	case "stalled":
		at = itemLastProgress(item)
	case "working":
		last := itemLastProgress(item)
		if last.IsZero() || now.Before(last.Add(sprintAwaitRunIdle)) {
			return sprintAwaitCandidate{}, false
		}
		state, at = "stalled", last.Add(sprintAwaitRunIdle)
	default:
		return sprintAwaitCandidate{}, false
	}
	return sprintAwaitCandidate{
		key: fmt.Sprintf("run/%s/%d/%s/%s/%s", queue, item.ID, item.Created, state, at),
		at:  at,
		event: sprintWaitEvent{
			Type: "run_" + state, SuggestedVerb: verb, StoryID: strings.TrimPrefix(link.Story, "todo:"),
			RunID: item.ID, Repo: link.Repo, Queue: queue, Message: item.Title,
		},
	}, true
}

func readSprintAwaitMail(owner, roomID string) ([]sprintAwaitCandidate, error) {
	if owner == "" {
		return nil, nil
	}
	restoreRoles, _ := memoizeHostRolesFor(owner)
	posts, other, _, err := bus.Unseen(owner, 0)
	restoreRoles()
	if err != nil {
		return nil, err
	}
	for _, p := range other {
		if p.Audience != nil && strings.EqualFold(strings.TrimSpace(p.Audience.Role), "conductor") {
			posts = append(posts, p)
		}
	}
	var events []sprintAwaitCandidate
	add := func(source string, seq int64, from, body string, at time.Time) {
		if strings.EqualFold(from, owner) {
			return
		}
		events = append(events, sprintAwaitCandidate{
			key: fmt.Sprintf("%s/%d", source, seq), at: at,
			event: sprintWaitEvent{Type: "manager_reply", SuggestedVerb: "reply", From: from, Message: body},
		})
	}
	for _, p := range posts {
		at, _ := parseInboxTime(p.At)
		add("mb", p.Seq, p.From, p.Body, at)
	}
	if roomID != "" {
		replies, _, _, _, err := meet.UnreadRecords(roomID, owner, 0)
		if err != nil {
			return nil, err
		}
		for _, p := range replies {
			add("meet/"+roomID, p.Seq, p.Event.Speaker, p.Event.Text, p.Event.TS)
		}
	}
	return events, nil
}

func detectSprintAwaitEvent(prev, cur *sprintWaitSnapshot, id int64) *sprintWaitEvent {
	seen := make(map[string]bool, len(prev.Actionable))
	for _, c := range prev.Actionable {
		seen[c.key] = true
	}
	var next sprintAwaitCandidate
	consider := func(c sprintAwaitCandidate) {
		if seen[c.key] {
			return
		}
		if next.event.Type == "" || (!c.at.IsZero() && (next.at.IsZero() || c.at.Before(next.at))) {
			next = c
		}
	}
	for i := min(len(prev.Thread), len(cur.Thread)); i < len(cur.Thread); i++ {
		c := cur.Thread[i]
		if c.Kind != "submit" || c.StoryID == "" {
			continue
		}
		consider(sprintAwaitCandidate{at: c.At, event: sprintWaitEvent{
			Type: "story_submitted", SuggestedVerb: "accept", StoryID: c.StoryID,
			EvidenceRef: c.EvidenceRef, From: c.Author, Message: c.Body,
		}})
	}
	for _, c := range cur.Actionable {
		consider(c)
	}
	if next.event.Type == "" {
		return nil
	}
	next.event.SchemaVersion, next.event.Sprint = sprintAwaitSchema, id
	if !next.at.IsZero() {
		next.event.At = next.at.UTC().Format(time.RFC3339Nano)
	}
	return &next.event
}

func writeSprintAwaitEvent(out io.Writer, ev *sprintWaitEvent) error {
	details := []string{ev.Type, fmt.Sprintf("sprint #%d", ev.Sprint)}
	if ev.StoryID != "" {
		details = append(details, "story "+ev.StoryID)
	}
	if ev.RunID != 0 {
		details = append(details, fmt.Sprintf("%s run #%d (%s)", ev.Repo, ev.RunID, ev.Queue))
	}
	if ev.EvidenceRef != "" {
		details = append(details, "evidence "+ev.EvidenceRef)
	}
	if ev.From != "" {
		details = append(details, "from "+ev.From)
	}
	_, err := fmt.Fprintf(out, "%s: %q (suggested: %s)\n", strings.Join(details, " · "), ev.Message, ev.SuggestedVerb)
	return err
}
