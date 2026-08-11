package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/room"
)

func TestAgentRosterEmptyIncludesCatalogFooter(t *testing.T) {
	useAgentsHome(t, t.TempDir())
	var out bytes.Buffer
	if err := renderAgentRoster(&out, false); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasSuffix(got, "To list all registered agents: bashy agents list\n") {
		t.Fatalf("missing exact footer: %q", got)
	}
	if strings.Contains(got, "working task") {
		t.Fatalf("empty roster contained work: %q", got)
	}
}

func TestAgentRosterFiltersToWorkingAssignments(t *testing.T) {
	home := t.TempDir()
	useAgentsHome(t, home)
	writeAgentsQueue(t, home, "demo-a1", agentsQueue{Root: "/work/demo", Items: []agentsQueueItem{
		{ID: 1, Title: "working task", State: "working", Tool: "codex", Owner: "codex-worker", StartedAt: time.Now().Add(-2 * time.Minute)},
		{ID: 2, Title: "paused task", State: "paused", Tool: "claude", Owner: "claude-worker"},
		{ID: 3, Title: "done task", State: "done", Tool: "agy", Owner: "agy-worker"},
	}, Stories: []agentsQueueStory{{ID: 9, Runs: []struct {
		Repo string `json:"repo"`
		ID   int64  `json:"id"`
	}{{Repo: "demo", ID: 1}}}}})
	var out bytes.Buffer
	if err := renderAgentRoster(&out, false); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"codex-worker", "demo", "#1", "#9", "working", "working task"} {
		if !strings.Contains(got, want) {
			t.Errorf("roster missing %q:\n%s", want, got)
		}
	}
	for _, absent := range []string{"paused task", "done task"} {
		if strings.Contains(got, absent) {
			t.Errorf("roster included inactive %q:\n%s", absent, got)
		}
	}
}

func TestAgentRosterJSON(t *testing.T) {
	home := t.TempDir()
	useAgentsHome(t, home)
	writeAgentsQueue(t, home, "demo-a1", agentsQueue{Root: "/work/demo", Items: []agentsQueueItem{{ID: 7, Title: "ship", State: "working", Tool: "codex"}}})
	var out bytes.Buffer
	if err := renderAgentRoster(&out, true); err != nil {
		t.Fatal(err)
	}
	var got agentsRoster
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != agentsRosterSchema || len(got.Assignments) != 1 || got.Assignments[0].Run != 7 {
		t.Fatalf("unexpected JSON: %#v", got)
	}
	if strings.Contains(out.String(), "To list all registered") {
		t.Fatal("JSON included plain footer")
	}
}

func TestAgentsListStillUsesRegisteredCatalog(t *testing.T) {
	useAgentsHome(t, t.TempDir())
	cmd := newAgentsRosterCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[") {
		t.Fatalf("agents list no longer emitted catalog JSON: %q", out.String())
	}
}

func TestAgentRosterReconcilesConductorWorkerAndReplacement(t *testing.T) {
	home, sprintDir, roomDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_SPRINT_DIR", sprintDir)
	t.Setenv("BASHY_ROOM_DIR", roomDir)
	useAgentsHome(t, home)
	now := time.Now().UTC()
	cutoff := now.Add(time.Hour)
	board := agentsSprintBoard{Stories: []agentsSprint{{
		ID: 49, Title: "POSIX utilities", Column: "doing",
		Lease: &agentsSprintLease{Holder: "replacement-conductor", At: now.Add(-time.Minute)},
		Runs:  []agentsSprintRun{{Repo: "posix", ID: 7}},
		Boxes: []agentsSprintBox{{StartedAt: now.Add(-10 * time.Minute), Cutoff: cutoff}},
	}}}
	writeJSONFile(t, filepath.Join(sprintDir, "queue.json"), board)
	q := agentsQueue{Root: "/campaign/posix", Items: []agentsQueueItem{{
		ID: 7, Title: "xargs", State: "working", Owner: "replacement-worker",
		Points: 3, WrapperPID: os.Getpid(), StartedAt: now.Add(-2 * time.Minute),
		LaunchSpec: &agentsLaunchSpec{Agent: "replacement-worker", MaxRuntime: 10 * time.Minute},
		Comments:   []agentsComment{{At: now.Add(-30 * time.Second), Kind: "progress"}},
	}}}
	if err := os.MkdirAll(filepath.Join(home, "weave", "posix"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSONFile(t, filepath.Join(home, "weave", "posix", "queue.json"), q)
	if err := room.Join(room.Card{ID: fmt.Sprintf("weave-7-%d", os.Getpid()), Tool: "codex", Binding: "codex:model", Mode: "weave", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.Leave(fmt.Sprintf("weave-7-%d", os.Getpid())) })

	assignments, err := reconciledAgentRoster()
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 2 {
		t.Fatalf("assignments = %#v, want conductor and worker", assignments)
	}
	if assignments[0].Role != "conductor" || assignments[0].Agent != "replacement-conductor" || assignments[0].Sprint != 49 {
		t.Fatalf("conductor assignment = %#v", assignments[0])
	}
	worker := assignments[1]
	if worker.Role != "worker" || worker.Agent != "replacement-worker" || worker.Owner != "replacement-worker" || worker.Run != 7 || worker.Sprint != 49 {
		t.Fatalf("worker assignment = %#v", worker)
	}
	if worker.Points != 3 || !worker.Deadline.Equal(now.Add(8*time.Minute)) || worker.Health != "healthy" {
		t.Fatalf("worker evidence = %#v", worker)
	}
	var jsonOut, humanOut bytes.Buffer
	if err := renderAgentRoster(&jsonOut, true); err != nil {
		t.Fatal(err)
	}
	var envelope agentsRoster
	if err := json.Unmarshal(jsonOut.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if err := renderAgentRoster(&humanOut, false); err != nil {
		t.Fatal(err)
	}
	for _, assignment := range envelope.Assignments {
		if !strings.Contains(humanOut.String(), assignment.Agent) || !strings.Contains(humanOut.String(), assignment.Health) {
			t.Fatalf("human roster disagrees with JSON assignment %#v:\n%s", assignment, humanOut.String())
		}
	}
}

func TestAgentRosterDerivesStaleAndBlockerAndDoesNotCrossPID(t *testing.T) {
	home, sprintDir, roomDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_SPRINT_DIR", sprintDir)
	t.Setenv("BASHY_ROOM_DIR", roomDir)
	useAgentsHome(t, home)
	queueDir := filepath.Join(home, "weave", "one")
	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		t.Fatal(err)
	}
	q := agentsQueue{Root: "/campaign/one", Items: []agentsQueueItem{
		{ID: 7, Title: "stale", State: "working", Owner: "old", WrapperPID: 999999, Comments: []agentsComment{{Kind: "system"}, {Kind: "progress", At: time.Now().Add(-time.Minute)}}},
		{ID: 7, Title: "blocked", State: "working", Owner: "new", Comments: []agentsComment{{Kind: "blocker", At: time.Now()}}},
		{ID: 8, Title: "allocated", State: "allocated", Owner: "launching"},
	}}
	writeJSONFile(t, filepath.Join(queueDir, "queue.json"), q)
	cardID := fmt.Sprintf("weave-7-%d", os.Getpid())
	if err := room.Join(room.Card{ID: cardID, Tool: "codex", Binding: "codex:model", Mode: "weave", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.Leave(cardID) })
	assignments, err := reconciledAgentRoster()
	if err != nil {
		t.Fatal(err)
	}
	byTitle := make(map[string]agentAssignment)
	for _, assignment := range assignments {
		byTitle[assignment.Title] = assignment
	}
	if byTitle["stale"].Health != "stale" || !strings.Contains(byTitle["stale"].HealthReason, "not alive") {
		t.Fatalf("stale projection = %#v", byTitle["stale"])
	}
	if byTitle["blocked"].Health != "blocked" {
		t.Fatalf("blocker projection = %#v", byTitle["blocked"])
	}
	if byTitle["allocated"].Health == "healthy" {
		t.Fatalf("zero-PID allocated item became healthy from another run's room card: %#v", byTitle["allocated"])
	}
}

func TestAgentRosterExcludesInactiveSprintLeasesAndOmitsZeroTimes(t *testing.T) {
	home, sprintDir, roomDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_SPRINT_DIR", sprintDir)
	t.Setenv("BASHY_ROOM_DIR", roomDir)
	useAgentsHome(t, home)
	now := time.Now().UTC().Add(-time.Hour)
	writeJSONFile(t, filepath.Join(sprintDir, "queue.json"), agentsSprintBoard{Stories: []agentsSprint{
		{ID: 4, Title: "finished", Column: "done", Lease: &agentsSprintLease{Holder: "old", At: now}},
		{ID: 15, Title: "queued", Column: "backlog", Lease: &agentsSprintLease{Holder: "queued", At: now}},
		{ID: 38, Title: "active but stale", Column: "doing", Lease: &agentsSprintLease{Holder: "stale-conductor", At: now}},
	}})
	assignments, err := reconciledAgentRoster()
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 || assignments[0].Sprint != 38 || assignments[0].Health != "stale" {
		t.Fatalf("lifecycle projection = %#v", assignments)
	}

	var out bytes.Buffer
	if err := renderAgentRoster(&out, true); err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Assignments []map[string]any `json:"assignments"`
	}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Assignments) != 1 {
		t.Fatalf("JSON assignments = %#v", raw.Assignments)
	}
	if _, ok := raw.Assignments[0]["deadline"]; ok {
		t.Fatalf("zero deadline was serialized: %s", out.String())
	}
	zero, err := json.Marshal(agentAssignment{Agent: "zero", Role: "worker", State: "allocated", Health: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"deadline", "last_progress"} {
		if strings.Contains(string(zero), `"`+absent+`"`) {
			t.Fatalf("zero %s was serialized: %s", absent, zero)
		}
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func useAgentsHome(t *testing.T, home string) {
	t.Helper()
	old := agentsHomeDir
	agentsHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { agentsHomeDir = old })
}

func writeAgentsQueue(t *testing.T, home, name string, q agentsQueue) {
	t.Helper()
	dir := filepath.Join(home, ".bashy", "weave", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "queue.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}
