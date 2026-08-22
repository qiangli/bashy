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
	if !strings.Contains(got, "LIVE 0") {
		t.Fatalf("empty roster did not report zero live assignments: %q", got)
	}
	if !strings.HasSuffix(got, "Track: bashy watch -n 2 bashy agents | JSON: bashy agents --json | Catalog: bashy agents list\n") {
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
		{ID: 1, Title: "working task", State: "working", Tool: "codex", Owner: "codex-worker", WrapperPID: os.Getpid(), StartedAt: time.Now().Add(-2 * time.Minute)},
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
	writeAgentsQueue(t, home, "demo-a1", agentsQueue{Root: "/work/demo", Items: []agentsQueueItem{{ID: 7, Title: "ship", State: "working", Tool: "codex", WrapperPID: os.Getpid()}}})
	var out bytes.Buffer
	if err := renderAgentRoster(&out, true); err != nil {
		t.Fatal(err)
	}
	var got agentsRoster
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != agentsRosterSchema || got.Summary.Live != 1 || len(got.Assignments) != 1 || got.Assignments[0].Run != 7 {
		t.Fatalf("unexpected JSON: %#v", got)
	}
	if strings.Contains(out.String(), "To list all registered") {
		t.Fatal("JSON included plain footer")
	}
}

func TestAgentRosterIncludesStandaloneRoomAssignments(t *testing.T) {
	home, sprintDir, roomDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_SPRINT_DIR", sprintDir)
	t.Setenv("BASHY_ROOM_DIR", roomDir)
	useAgentsHome(t, home)
	joined := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	cards := []room.Card{
		{
			ID: "Beatrix-invoke", Principal: "conductor", Tool: "claude", Model: "opus4.8",
			Binding: "claude:opus4.8", Nick: "Beatrix", Band: 4, Mode: "oneshot",
			Role: "reviewer", Task: "review assignment visibility", PID: os.Getpid(), Cwd: "/work/bashy", Joined: joined.Format(time.RFC3339),
		},
		{
			ID: "codex-gpt-5-5", Principal: "operator", Tool: "codex", Model: "gpt-5.5",
			Binding: "codex:gpt-5.5", Mode: "interactive", PID: os.Getpid(), Cwd: "/work/coreutils",
		},
	}
	for _, card := range cards {
		if err := room.Join(card); err != nil {
			t.Fatal(err)
		}
		card := card
		t.Cleanup(func() { room.Leave(card.ID) })
	}

	assignments, err := reconciledAgentRoster()
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 2 {
		t.Fatalf("assignments = %#v, want both named and ad-hoc room work", assignments)
	}
	byBinding := make(map[string]agentAssignment)
	for _, assignment := range assignments {
		byBinding[assignment.Binding] = assignment
	}
	named := byBinding["claude:opus4.8"]
	if named.Agent != "Beatrix" || named.Mode != "oneshot" || named.InvocationRole != "reviewer" || named.Source != "room" || named.Title != "review assignment visibility" {
		t.Fatalf("named invocation = %#v", named)
	}
	if named.Owner != "conductor" || named.PID != os.Getpid() || named.Repo != "bashy" || named.LastProgress.Before(joined) {
		t.Fatalf("named invocation evidence = %#v", named)
	}
	adhoc := byBinding["codex:gpt-5.5"]
	if adhoc.Agent != "codex:gpt-5.5" || adhoc.Mode != "interactive" || adhoc.Source != "room" || adhoc.Repo != "coreutils" {
		t.Fatalf("ad-hoc invocation = %#v", adhoc)
	}
	if !strings.Contains(adhoc.Title, "unlabeled live interactive") {
		t.Fatalf("ad-hoc work label = %q", adhoc.Title)
	}

	var out bytes.Buffer
	if err := renderAgentRoster(&out, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Beatrix", "review assignment visibility", "oneshot", "reviewer", "claude:opus4.8", "room"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("human roster missing %q:\n%s", want, out.String())
		}
	}
}

func TestAgentRosterTreatsRawShellPresenceAsIdleNotAssignedWork(t *testing.T) {
	home := t.TempDir()
	useAgentsHome(t, home)
	id := fmt.Sprintf("shell:codex:%d", os.Getpid())
	if err := room.Join(room.Card{ID: id, Tool: "codex", Binding: "codex", Mode: shellSessionMode, PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.Leave(id) })

	var live bytes.Buffer
	if err := renderAgentRoster(&live, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(live.String(), "LIVE 0") || strings.Contains(live.String(), "unlabeled live shell") {
		t.Fatalf("raw shell presence was reported as assigned work: %s", live.String())
	}

	var all bytes.Buffer
	if err := renderAgentRosterView(&all, false, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(all.String(), "raw shell presence has no published assignment") {
		t.Fatalf("--all did not retain auditable shell presence: %s", all.String())
	}
}

func TestAgentsTrackPublishesHeartbeatsExpiresAndStopsExternalWork(t *testing.T) {
	home := t.TempDir()
	useAgentsHome(t, home)
	run := func(args ...string) string {
		t.Helper()
		cmd := newAgentsRosterCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("agents %v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}

	start := run("track", "start", "pair-stage", "--agent", "Parfit", "--binding", "codex:gpt-5.6-sol",
		"--role", "worker", "--task", "repair durable DO launcher", "--owner-pid", fmt.Sprint(os.Getpid()), "--ttl", "1h")
	if !strings.Contains(start, "TRACKING pair-stage") {
		t.Fatalf("start output = %q", start)
	}
	assignments, err := reconciledAgentRoster()
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 || assignments[0].Agent != "Parfit" || assignments[0].Source != externalAssignmentMode || assignments[0].Health != "healthy" {
		t.Fatalf("tracked assignment = %#v", assignments)
	}
	joined := assignments[0].Age

	heartbeat := run("track", "heartbeat", "pair-stage", "--owner-pid", fmt.Sprint(os.Getpid()), "--ttl", "2h")
	if !strings.Contains(heartbeat, "HEARTBEAT pair-stage") {
		t.Fatalf("heartbeat output = %q", heartbeat)
	}
	assignments, err = reconciledAgentRoster()
	if err != nil || len(assignments) != 1 || assignments[0].Age != joined {
		t.Fatalf("heartbeat changed assignment identity/start: %#v err=%v", assignments, err)
	}

	card, ok, err := room.Find("external-pair-stage")
	if err != nil || !ok {
		t.Fatalf("find tracked card: ok=%v err=%v", ok, err)
	}
	card.LeaseUntil = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	assignments, err = reconciledAgentRoster()
	if err != nil || len(assignments) != 1 || assignments[0].Health != "stale" {
		t.Fatalf("expired tracked assignment = %#v err=%v", assignments, err)
	}

	stop := run("track", "stop", "pair-stage", "--owner-pid", fmt.Sprint(os.Getpid()))
	if !strings.Contains(stop, "STOPPED pair-stage") {
		t.Fatalf("stop output = %q", stop)
	}
	if _, ok, _ := room.Find("external-pair-stage"); ok {
		t.Fatal("stopped tracked assignment remains in room")
	}
}

func TestAgentRosterDoesNotDuplicateWeaveRoomCard(t *testing.T) {
	home, sprintDir, roomDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_SPRINT_DIR", sprintDir)
	t.Setenv("BASHY_ROOM_DIR", roomDir)
	useAgentsHome(t, home)
	writeJSONFile(t, filepath.Join(home, "weave", "demo-a1", "queue.json"), agentsQueue{Root: "/work/demo", Items: []agentsQueueItem{{
		ID: 91, Title: "visible once", State: "working", Owner: "Beatrix", WrapperPID: os.Getpid(), StartedAt: time.Now(),
	}}})
	cardID := fmt.Sprintf("weave-91-%d", os.Getpid())
	if err := room.Join(room.Card{
		ID: cardID, Principal: "conductor", Tool: "claude", Model: "opus4.8", Binding: "claude:opus4.8",
		Nick: "Beatrix", Mode: "weave", Task: "#91 visible once", PID: os.Getpid(), Cwd: "/work/demo",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.Leave(cardID) })

	assignments, err := reconciledAgentRoster()
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 {
		t.Fatalf("assignments = %#v, want one reconciled weave assignment", assignments)
	}
	got := assignments[0]
	if got.Run != 91 || got.Source != "weave" || got.Mode != "weave" || got.Binding != "claude:opus4.8" || got.PID != os.Getpid() {
		t.Fatalf("reconciled weave assignment = %#v", got)
	}
}

func TestRoomRepoUsesRepositoryRootFromNestedCwd(t *testing.T) {
	root := filepath.Join(t.TempDir(), "visible-repo")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "internal", "agentos")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := roomRepo(nested); got != "visible-repo" {
		t.Fatalf("roomRepo(%q) = %q, want repository root label", nested, got)
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

func TestAgentsAllAppliesToAssignmentView(t *testing.T) {
	home, sprintDir, roomDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_SPRINT_DIR", sprintDir)
	t.Setenv("BASHY_ROOM_DIR", roomDir)
	useAgentsHome(t, home)
	writeJSONFile(t, filepath.Join(sprintDir, "queue.json"), agentsSprintBoard{Stories: []agentsSprint{{
		ID: 9, Title: "stale", Column: "doing",
		Lease: &agentsSprintLease{Holder: "stale-conductor", At: time.Now().Add(-time.Hour)},
	}}})
	cmd := newAgentsRosterCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--all", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "stale-conductor") {
		t.Fatalf("agents --all did not expose stale assignment: %q", out.String())
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
		{ID: 7, Title: "blocked", State: "working", Owner: "new", WrapperPID: os.Getpid(), Comments: []agentsComment{{Kind: "blocker", At: time.Now()}}},
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
	if byTitle["allocated"].Health != "orphaned" {
		t.Fatalf("zero-PID allocated item = %#v, want orphaned", byTitle["allocated"])
	}
}

func TestAgentRosterDefaultHidesStaleAndAllShowsIt(t *testing.T) {
	home, sprintDir, roomDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_SPRINT_DIR", sprintDir)
	t.Setenv("BASHY_ROOM_DIR", roomDir)
	useAgentsHome(t, home)
	writeJSONFile(t, filepath.Join(sprintDir, "queue.json"), agentsSprintBoard{Stories: []agentsSprint{{
		ID: 54, Title: "expired conductor", Column: "doing",
		Lease: &agentsSprintLease{Holder: "old-conductor", At: time.Now().Add(-time.Hour)},
	}}})
	queueDir := filepath.Join(home, "weave", "demo-a1")
	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSONFile(t, filepath.Join(queueDir, "queue.json"), agentsQueue{Root: "/work/demo", Items: []agentsQueueItem{{
		ID: 8, Title: "orphan allocation", State: "allocated", Owner: "lost-worker",
	}}})

	var live bytes.Buffer
	if err := renderAgentRoster(&live, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(live.String(), "LIVE 0") || !strings.Contains(live.String(), "STALE 1") || !strings.Contains(live.String(), "ORPHANED 1") {
		t.Fatalf("default summary = %q", live.String())
	}
	for _, hidden := range []string{"old-conductor", "lost-worker"} {
		if strings.Contains(live.String(), hidden) {
			t.Fatalf("default roster exposed stale assignment %q: %s", hidden, live.String())
		}
	}

	var all bytes.Buffer
	if err := renderAgentRosterView(&all, false, true); err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{"old-conductor", "lost-worker"} {
		if !strings.Contains(all.String(), visible) {
			t.Fatalf("--all roster omitted %q: %s", visible, all.String())
		}
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
	if err := renderAgentRosterView(&out, true, true); err != nil {
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
	t.Setenv("BASHY_ROOM_DIR", filepath.Join(home, "room"))
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
