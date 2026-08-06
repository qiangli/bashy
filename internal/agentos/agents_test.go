package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
