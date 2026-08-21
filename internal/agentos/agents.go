// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// The `agents` view is a reconciled projection of the three local execution
// stores: weave queues (workers), the sprint board (conductor leases), and the
// host room (process liveness). None of those stores alone is enough to say
// that an agent is really working. In particular, a queue item can survive a
// killed wrapper and a sprint lease can outlive its conductor.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/room"
	"github.com/spf13/cobra"
)

// The envelope name is kept stable; the reconciled fields are additive so an
// existing `agents --json` consumer can continue reading Assignments.
const agentsRosterSchema = "bashy-agents-v1"

type agentsQueue struct {
	Root    string             `json:"root"`
	Items   []agentsQueueItem  `json:"items"`
	Stories []agentsQueueStory `json:"stories"`
}

type agentsQueueItem struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	State       string            `json:"state"`
	Tool        string            `json:"tool"`
	Owner       string            `json:"owner"`
	Points      int               `json:"points,omitempty"`
	Created     time.Time         `json:"created"`
	StartedAt   time.Time         `json:"started_at"`
	WrapperPID  int               `json:"wrapper_pid,omitempty"`
	Stale       bool              `json:"stale,omitempty"`
	Blocked     bool              `json:"blocked,omitempty"`
	LaunchPhase string            `json:"launch_phase,omitempty"`
	LaunchSpec  *agentsLaunchSpec `json:"launch_spec,omitempty"`
	Comments    []agentsComment   `json:"comments,omitempty"`
}

type agentsLaunchSpec struct {
	Agent      string        `json:"agent,omitempty"`
	Tool       string        `json:"tool,omitempty"`
	MaxRuntime time.Duration `json:"max_runtime,omitempty"`
}

type agentsComment struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind,omitempty"`
}

type agentsQueueStory struct {
	ID   int64 `json:"id"`
	Runs []struct {
		Repo string `json:"repo"`
		ID   int64  `json:"id"`
	} `json:"runs"`
}

type agentsSprintBoard struct {
	Stories []agentsSprint `json:"stories"`
}

type agentsSprint struct {
	ID      int64              `json:"id"`
	Title   string             `json:"title"`
	Column  string             `json:"column"`
	Lease   *agentsSprintLease `json:"lease,omitempty"`
	Runs    []agentsSprintRun  `json:"runs,omitempty"`
	Boxes   []agentsSprintBox  `json:"boxes,omitempty"`
	Updated time.Time          `json:"updated_at,omitempty"`
}

type agentsSprintLease struct {
	Holder string    `json:"holder"`
	At     time.Time `json:"at"`
}

type agentsSprintRun struct {
	Repo string `json:"repo"`
	ID   int64  `json:"id"`
}

type agentsSprintBox struct {
	StartedAt time.Time  `json:"started_at"`
	Cutoff    time.Time  `json:"cutoff"`
	StoppedAt *time.Time `json:"stopped_at,omitempty"`
}

// agentAssignment is deliberately a union: conductors have a sprint but no
// run, workers have a run and may have a sprint. Zero-valued fields are omitted
// in JSON so old consumers can continue reading the assignment fields.
type agentAssignment struct {
	Agent          string    `json:"agent"`
	Binding        string    `json:"binding,omitempty"`
	Role           string    `json:"role"`
	InvocationRole string    `json:"invocation_role,omitempty"`
	Mode           string    `json:"mode,omitempty"`
	Source         string    `json:"source,omitempty"`
	Owner          string    `json:"owner,omitempty"`
	PID            int       `json:"pid,omitempty"`
	Repo           string    `json:"repo"`
	Run            int64     `json:"run"`
	Sprint         int64     `json:"sprint,omitempty"`
	Points         int       `json:"points,omitempty"`
	Deadline       time.Time `json:"deadline,omitempty"`
	LastProgress   time.Time `json:"last_progress,omitempty"`
	State          string    `json:"state"`
	Health         string    `json:"health"`
	HealthReason   string    `json:"health_reason,omitempty"`
	Age            string    `json:"age,omitempty"`
	Title          string    `json:"title"`
}

type agentsRoster struct {
	SchemaVersion string             `json:"schema_version"`
	Summary       agentRosterSummary `json:"summary"`
	Assignments   []agentAssignment  `json:"assignments"`
}

type agentRosterSummary struct {
	Live         int `json:"live"`
	Blocked      int `json:"blocked"`
	Inconsistent int `json:"inconsistent"`
	Stale        int `json:"stale"`
	Orphaned     int `json:"orphaned"`
}

// encoding/json does not omit a zero time.Time with `omitempty` because it is
// a struct. Keep the internal projection convenient for comparisons while
// making the public envelope honest about absent evidence.
func (a agentAssignment) MarshalJSON() ([]byte, error) {
	type wire struct {
		Agent          string     `json:"agent"`
		Binding        string     `json:"binding,omitempty"`
		Role           string     `json:"role"`
		InvocationRole string     `json:"invocation_role,omitempty"`
		Mode           string     `json:"mode,omitempty"`
		Source         string     `json:"source,omitempty"`
		Owner          string     `json:"owner,omitempty"`
		PID            int        `json:"pid,omitempty"`
		Repo           string     `json:"repo"`
		Run            int64      `json:"run"`
		Sprint         int64      `json:"sprint,omitempty"`
		Points         int        `json:"points,omitempty"`
		Deadline       *time.Time `json:"deadline,omitempty"`
		LastProgress   *time.Time `json:"last_progress,omitempty"`
		State          string     `json:"state"`
		Health         string     `json:"health"`
		HealthReason   string     `json:"health_reason,omitempty"`
		Age            string     `json:"age,omitempty"`
		Title          string     `json:"title"`
	}
	var deadline, progress *time.Time
	if !a.Deadline.IsZero() {
		v := a.Deadline
		deadline = &v
	}
	if !a.LastProgress.IsZero() {
		v := a.LastProgress
		progress = &v
	}
	return json.Marshal(wire{
		Agent: a.Agent, Binding: a.Binding, Role: a.Role, InvocationRole: a.InvocationRole, Mode: a.Mode, Source: a.Source,
		Owner: a.Owner, PID: a.PID, Repo: a.Repo, Run: a.Run,
		Sprint: a.Sprint, Points: a.Points, Deadline: deadline, LastProgress: progress,
		State: a.State, Health: a.Health, HealthReason: a.HealthReason, Age: a.Age, Title: a.Title,
	})
}

var agentsHomeDir = os.UserHomeDir

func newAgentsRosterCmd(opts ...fleet.Option) *cobra.Command {
	cmd := fleet.NewAgentsCmd(opts...)
	cmd.Short = "Show every live agent assignment (use `agents list` for the catalog)"
	cmd.Long = "Show all live named and ad-hoc work reconciled from sprint leases, weave queues, and room membership, including one-shot invocations and interactive sessions.\n\n" +
		"Stale and orphaned records are counted but hidden by default; use --all to inspect them. " +
		"Use --json for the machine-readable workload view. Use `bashy agents list` to list all registered agent bindings."
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		for _, name := range []string{"band", "min-band"} {
			if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
				return fmt.Errorf("--%s applies to `bashy agents list`, not the active-assignment view", name)
			}
		}
		showAll, err := cmd.Flags().GetBool("all")
		if err != nil {
			return err
		}
		return renderAgentRosterView(cmd.OutOrStdout(), cmd.Flags().Lookup("json").Changed, showAll)
	}
	return cmd
}

func renderAgentRoster(w io.Writer, asJSON bool) error {
	return renderAgentRosterView(w, asJSON, false)
}

func renderAgentRosterView(w io.Writer, asJSON, showAll bool) error {
	assignments, err := reconciledAgentRoster()
	if err != nil {
		return err
	}
	summary := summarizeAgentRoster(assignments)
	visible := assignments
	if !showAll {
		visible = visible[:0]
		for _, assignment := range assignments {
			if assignmentLive(assignment) {
				visible = append(visible, assignment)
			}
		}
	}
	if asJSON {
		return json.NewEncoder(w).Encode(agentsRoster{SchemaVersion: agentsRosterSchema, Summary: summary, Assignments: visible})
	}
	fmt.Fprintf(w, "LIVE %d (blocked %d, inconsistent %d) | STALE %d | ORPHANED %d\n", summary.Live, summary.Blocked, summary.Inconsistent, summary.Stale, summary.Orphaned)
	if !showAll && len(visible) == 0 && (summary.Stale > 0 || summary.Orphaned > 0) {
		fmt.Fprintln(w, "No live assignments. Inspect stale records with: bashy agents --all")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	// Keep the original columns first. Consumers commonly use the human view
	// in a terminal copy/paste workflow; reconciled fields follow additively.
	fmt.Fprintln(tw, "AGENT\tOWNER\tREPO\tRUN\tSPRINT\tSTATE\tAGE\tWORK\tROLE\tPOINTS\tDEADLINE\tHEALTH\tLAST PROGRESS\tMODE\tROUTED AS\tPID\tBINDING\tSOURCE")
	for _, a := range visible {
		run, sprint := "-", "-"
		if a.Run != 0 {
			run = fmt.Sprintf("#%d", a.Run)
		}
		if a.Sprint != 0 {
			sprint = fmt.Sprintf("#%d", a.Sprint)
		}
		owner := a.Owner
		if owner == "" {
			owner = "-"
		}
		points := "-"
		if a.Points != 0 {
			points = strconv.Itoa(a.Points)
		}
		deadline := "-"
		if !a.Deadline.IsZero() {
			deadline = a.Deadline.Local().Format("15:04:05")
		}
		last := "-"
		if !a.LastProgress.IsZero() {
			last = assignmentAgeAt(a.LastProgress, time.Now()) + " ago"
		}
		title := a.Title
		if title == "" {
			title = "-"
		}
		pid := "-"
		if a.PID > 0 {
			pid = strconv.Itoa(a.PID)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			a.Agent, owner, dash(a.Repo), run, sprint, dash(a.State), dash(a.Age), title,
			a.Role, points, deadline, healthLabel(a), last, dash(a.Mode), dash(a.InvocationRole), pid, dash(a.Binding), dash(a.Source))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, "Track: bashy watch -n 2 bashy agents | JSON: bashy agents --json | Catalog: bashy agents list")
	return err
}

func summarizeAgentRoster(assignments []agentAssignment) agentRosterSummary {
	var summary agentRosterSummary
	for _, assignment := range assignments {
		if assignmentLive(assignment) {
			summary.Live++
		}
		switch assignment.Health {
		case "blocked":
			summary.Blocked++
		case "inconsistent":
			summary.Inconsistent++
		case "stale":
			summary.Stale++
		case "orphaned", "unknown":
			summary.Orphaned++
		}
	}
	return summary
}

func assignmentLive(assignment agentAssignment) bool {
	switch assignment.Health {
	case "healthy", "blocked", "inconsistent":
		return true
	default:
		return false
	}
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func healthLabel(a agentAssignment) string {
	if a.HealthReason == "" {
		return a.Health
	}
	return a.Health + " (" + a.HealthReason + ")"
}

// reconciledAgentRoster reads all stores before rendering either output mode.
// This is intentionally one function: JSON and human output cannot disagree
// about who is assigned when they share the same snapshot.
func reconciledAgentRoster() ([]agentAssignment, error) {
	home, err := agentsHomeDir()
	if err != nil {
		return nil, err
	}
	weaveRoots, sprintRoot := agentStoreRoots(home)
	roomMembers, err := room.Members()
	if err != nil {
		return nil, err
	}
	roomByRunPID := make(map[string][]room.Card)
	for _, card := range roomMembers {
		if id, pid, ok := roomRunIdentity(card.ID); ok {
			roomByRunPID[roomRunPIDKey(id, pid)] = append(roomByRunPID[roomRunPIDKey(id, pid)], card)
		}
	}
	consumedRoomCards := make(map[string]bool)

	sprints, err := loadAgentSprints(sprintRoot)
	if err != nil {
		return nil, err
	}
	byRun := make(map[string]agentsSprint)
	var out []agentAssignment
	now := time.Now()
	for _, sprint := range sprints {
		for _, run := range sprint.Runs {
			byRun[runKey(run.Repo, run.ID)] = sprint
		}
		if !activeSprintColumn(sprint.Column) || sprint.Lease == nil || strings.TrimSpace(sprint.Lease.Holder) == "" {
			continue
		}
		a := agentAssignment{
			Agent: sprint.Lease.Holder, Role: "conductor", Owner: sprint.Lease.Holder,
			Sprint: sprint.ID, State: sprint.Column, Title: sprint.Title,
			Health: "healthy", LastProgress: sprint.Lease.At, Source: "sprint",
		}
		if a.State == "" {
			a.State = "assigned"
		}
		if deadline := sprintDeadline(sprint, now); !deadline.IsZero() {
			a.Deadline = deadline
		}
		if sprint.Lease.At.IsZero() {
			a.Health, a.HealthReason = "orphaned", "missing conductor heartbeat"
		} else if now.Sub(sprint.Lease.At) > 30*time.Minute {
			a.Health, a.HealthReason = "stale", "conductor lease heartbeat expired"
		}
		a.Age = assignmentAgeAt(sprint.Lease.At, now)
		out = append(out, a)
	}

	for _, base := range weaveRoots {
		entries, err := os.ReadDir(base)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(base, entry.Name(), "queue.json")
			var q agentsQueue
			found, err := readAgentJSON(path, &q)
			if err != nil {
				return nil, fmt.Errorf("read weave queue %s: %w", path, err)
			}
			if !found {
				continue
			}
			repo := filepath.Base(q.Root)
			if repo == "." || repo == "" {
				repo = entry.Name()
			}
			// Older per-repo queues carried only the sprint/run link. Keep that
			// link visible when the global sprint board is absent; the board is
			// still preferred because it supplies the conductor/deadline facts.
			for _, story := range q.Stories {
				for _, run := range story.Runs {
					key := runKey(run.Repo, run.ID)
					if _, ok := byRun[key]; !ok {
						byRun[key] = agentsSprint{ID: story.ID}
					}
				}
			}
			for _, item := range q.Items {
				if !agentItemActive(item) {
					continue
				}
				sprint, hasSprint := byRun[runKey(repo, item.ID)]
				a := agentAssignment{
					Agent: agentItemName(item), Role: "worker", Owner: item.Owner,
					Repo: repo, Run: item.ID, State: item.State, Title: item.Title,
					Points: item.Points, LastProgress: itemLastProgress(item), Source: "weave",
				}
				if hasSprint {
					a.Sprint = sprint.ID
				}
				// A worker's deadline is its own bounded launch runtime. The
				// sprint box is a conductor review cutoff, not a worker cap.
				if !item.StartedAt.IsZero() && item.LaunchSpec != nil && item.LaunchSpec.MaxRuntime > 0 {
					a.Deadline = item.StartedAt.Add(item.LaunchSpec.MaxRuntime)
				}
				a.Age = assignmentAgeAt(a.LastProgress, now)
				cards := roomByRunPID[roomRunPIDKey(item.ID, item.WrapperPID)]
				a.Health, a.HealthReason = workerHealth(item, cards)
				for _, card := range cards {
					consumedRoomCards[card.ID] = true
					if a.Binding == "" {
						a.Binding = card.Binding
						a.Mode = card.Mode
						a.InvocationRole = card.Role
						a.PID = card.PID
					}
				}
				out = append(out, a)
			}
		}
	}

	// Room membership is the common denominator across launch paths. Project
	// every live card that was not already represented by a weave queue item so
	// short-lived invocations, interactive sessions, meet/foreman workers, and
	// ad-hoc tool:model launches are visible through the same `bashy agents`
	// surface. A live process with no durable sprint/run is still consuming an
	// agent identity and provider capacity; hiding it defeats workload routing.
	for _, card := range roomMembers {
		if consumedRoomCards[card.ID] {
			continue
		}
		joined := parseRoomJoined(card.Joined)
		a := agentAssignment{
			Agent:          roomAgentName(card),
			Binding:        card.Binding,
			Role:           "worker",
			InvocationRole: card.Role,
			Mode:           card.Mode,
			Source:         "room",
			Owner:          card.Principal,
			PID:            card.PID,
			Repo:           roomRepo(card.Cwd),
			State:          "working",
			LastProgress:   joined,
			Health:         "healthy",
			HealthReason:   "live room member",
			Age:            assignmentAgeAt(joined, now),
			Title:          strings.TrimSpace(card.Task),
		}
		if a.Title == "" {
			a.Title = "unlabeled live " + dash(card.Mode)
		}
		if strings.EqualFold(card.Mode, "weave") {
			a.Health = "inconsistent"
			a.HealthReason = "live weave room member has no active queue assignment"
		}
		out = append(out, a)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role == "conductor"
		}
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		if out[i].Sprint != out[j].Sprint {
			return out[i].Sprint < out[j].Sprint
		}
		if out[i].Run != out[j].Run {
			return out[i].Run < out[j].Run
		}
		return out[i].Agent < out[j].Agent
	})
	return out, nil
}

func parseRoomJoined(joined string) time.Time {
	when, _ := time.Parse(time.RFC3339, strings.TrimSpace(joined))
	return when
}

func roomAgentName(card room.Card) string {
	if name := strings.TrimSpace(card.Nick); name != "" {
		return name
	}
	if binding := strings.TrimSpace(card.Binding); binding != "" {
		return binding
	}
	if id := strings.TrimSpace(card.ID); id != "" {
		return id
	}
	return "?"
}

func roomRepo(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}
	clean := filepath.Clean(cwd)
	for dir := clean; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return filepath.Base(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return filepath.Base(clean)
}

func activeSprintColumn(column string) bool {
	switch strings.ToLower(strings.TrimSpace(column)) {
	case "doing", "review":
		return true
	default:
		return false
	}
}

func agentStoreRoots(home string) ([]string, string) {
	if configured := strings.TrimSpace(os.Getenv("BASHY_HOME")); configured != "" {
		return []string{filepath.Join(configured, "weave")}, agentSprintRoot(configured, home)
	}
	return []string{
		filepath.Join(home, ".bashy", "weave"),
		filepath.Join(home, ".agents", "weave"),
		filepath.Join(home, ".agents", "ycode", "weave"),
	}, agentSprintRoot("", home)
}

func agentSprintRoot(configuredHome, home string) string {
	if dir := strings.TrimSpace(os.Getenv("BASHY_SPRINT_DIR")); dir != "" {
		return dir
	}
	if configuredHome != "" {
		return filepath.Join(configuredHome, "sprint")
	}
	return filepath.Join(home, ".bashy", "sprint")
}

func loadAgentSprints(root string) ([]agentsSprint, error) {
	var board agentsSprintBoard
	found, err := readAgentJSON(filepath.Join(root, "queue.json"), &board)
	if err != nil {
		return nil, fmt.Errorf("read sprint board: %w", err)
	}
	if !found {
		return nil, nil
	}
	return board.Stories, nil
}

func readAgentJSON(path string, dst any) (bool, error) {
	for attempt := 0; attempt < 3; attempt++ {
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return true, err
		}
		if err := json.Unmarshal(b, dst); err == nil {
			return true, nil
		} else if attempt == 2 {
			return true, err
		}
		time.Sleep(time.Millisecond)
	}
	return false, nil
}

func runKey(repo string, id int64) string {
	return strings.ToLower(strings.TrimSpace(repo)) + "/" + strconv.FormatInt(id, 10)
}

func agentItemActive(item agentsQueueItem) bool {
	switch item.State {
	case "allocated", "working", "finalizing":
		return true
	case "todo":
		return item.Owner != "" || item.Tool != "" || item.LaunchSpec != nil
	default:
		return false
	}
}

func agentItemName(item agentsQueueItem) string {
	if item.Owner != "" {
		return item.Owner
	}
	if item.LaunchSpec != nil && item.LaunchSpec.Agent != "" {
		return item.LaunchSpec.Agent
	}
	if item.Tool != "" {
		return item.Tool
	}
	return "?"
}

func itemLastProgress(item agentsQueueItem) time.Time {
	for i := len(item.Comments) - 1; i >= 0; i-- {
		if relevantWorkerComment(item.Comments[i].Kind) && !item.Comments[i].At.IsZero() {
			return item.Comments[i].At
		}
	}
	if !item.StartedAt.IsZero() {
		return item.StartedAt
	}
	return item.Created
}

func sprintDeadline(s agentsSprint, now time.Time) time.Time {
	for i := len(s.Boxes) - 1; i >= 0; i-- {
		box := s.Boxes[i]
		if !box.StartedAt.IsZero() && box.StoppedAt == nil {
			return box.Cutoff
		}
	}
	return time.Time{}
}

func roomRunID(id string) (int64, bool) {
	run, _, ok := roomRunIdentity(id)
	return run, ok
}

func roomRunIdentity(id string) (int64, int, bool) {
	parts := strings.Split(id, "-")
	if len(parts) < 3 || parts[0] != "weave" {
		return 0, 0, false
	}
	run, runErr := strconv.ParseInt(parts[1], 10, 64)
	pid, pidErr := strconv.Atoi(parts[2])
	return run, pid, runErr == nil && pidErr == nil && pid > 0
}

func roomRunPIDKey(run int64, pid int) string {
	return strconv.FormatInt(run, 10) + "/" + strconv.Itoa(pid)
}

func relevantWorkerComment(kind string) bool {
	switch kind {
	case "blocker", "progress", "decision", "review":
		return true
	default:
		return false
	}
}

func workerBlocked(item agentsQueueItem) bool {
	for i := len(item.Comments) - 1; i >= 0; i-- {
		if !relevantWorkerComment(item.Comments[i].Kind) {
			continue
		}
		return item.Comments[i].Kind == "blocker"
	}
	return false
}

func workerHealth(item agentsQueueItem, cards []room.Card) (string, string) {
	if item.WrapperPID > 0 && !room.PidAlive(item.WrapperPID) {
		return "stale", fmt.Sprintf("worker wrapper pid %d is not alive", item.WrapperPID)
	}
	if item.WrapperPID <= 0 {
		return "orphaned", "active queue item has no worker wrapper heartbeat"
	}
	if workerBlocked(item) {
		return "blocked", "latest worker note is a blocker"
	}
	if len(cards) > 0 {
		if item.WrapperPID > 0 {
			for _, card := range cards {
				if card.PID == item.WrapperPID {
					return "healthy", "room member and wrapper are live"
				}
			}
			return "inconsistent", "room member pid differs from queue wrapper"
		}
		return "healthy", "room member is live"
	}
	if item.WrapperPID > 0 {
		return "inconsistent", "live wrapper has no room member"
	}
	return "unknown", "no worker heartbeat recorded"
}

func assignmentAgeAt(then, now time.Time) string {
	if then.IsZero() || then.After(now) {
		return ""
	}
	d := now.Sub(then).Round(time.Second)
	if d >= time.Hour {
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// Kept for callers/tests that used the old helper directly.
func activeAgentAssignments() ([]agentAssignment, error) { return reconciledAgentRoster() }

func assignmentAge(item agentsQueueItem, now time.Time) string {
	return assignmentAgeAt(itemLastProgress(item), now)
}
