package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/room"
)

func TestWorkerStatusReadsCurrentSessionTurn(t *testing.T) {
	now := time.Now().UTC()
	joined := now.Add(-time.Minute)
	path := filepath.Join(t.TempDir(), "events.jsonl")
	stamp := func(at time.Time) string { return at.Format(time.RFC3339Nano) }
	events := `{"type":"turn.end","timestamp":"` + stamp(joined.Add(-time.Second)) + `","data":{"status":"ok"}}` + "\n" +
		`{"type":"turn.start","timestamp":"` + stamp(now.Add(-10*time.Second)) + `"}` + "\n" +
		`{"type":"turn.end","timestamp":"` + stamp(now.Add(-time.Second)) + `","data":{"status":"ok"}}`
	if err := os.WriteFile(path, []byte(events), 0o600); err != nil {
		t.Fatal(err)
	}
	card := room.Card{ID: "demo", Joined: stamp(joined), EventsPath: path}
	catalog := fleet.New()
	worker := readWorkerStatus(card, catalog, now)
	if worker.State != "working" || worker.Limit != "unavailable" {
		t.Fatalf("torn turn.end changed the current reading: %+v", worker)
	}
	if err := os.WriteFile(path, []byte(events+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worker = readWorkerStatus(card, catalog, now)
	if worker.State != "turn-ended" {
		t.Fatalf("completed turn.end did not change the current reading: %+v", worker)
	}
}

func TestWorkerStatusTerminalLimitRequiresCurrentEvidence(t *testing.T) {
	now := time.Date(2026, 8, 1, 20, 0, 0, 0, time.Local)
	path := filepath.Join(t.TempDir(), "session.log")
	catalog := fleet.New()
	card := room.Card{ID: "demo", Tool: "codex", Joined: now.Add(-time.Hour).Format(time.RFC3339), LogPath: path}
	setLog := func(line string, at time.Time) {
		t.Helper()
		if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}

	setLog("You've hit your usage limit. Try again at 8:30 PM.", now.Add(-time.Second))
	worker := readWorkerStatus(card, catalog, now)
	wantReset := now.Add(30 * time.Minute)
	if worker.Limit != "exhausted" || worker.ResetAt == nil || !worker.ResetAt.Equal(wantReset) {
		t.Fatalf("fresh terminal limit missing: %+v", worker)
	}

	for _, tc := range []struct {
		name string
		line string
		at   time.Time
	}{
		{"stale", "You've hit your usage limit. Try again at 8:30 PM.", now.Add(-3 * time.Minute)},
		{"expired", "You've hit your usage limit. Try again at 8:00 PM.", now.Add(-time.Second)},
		{"superseded", "You've hit your usage limit. Try again at 8:30 PM.\nWorking again", now.Add(-time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setLog(tc.line, tc.at)
			worker := readWorkerStatus(card, catalog, now)
			if worker.Limit != "unavailable" || worker.ResetAt != nil {
				t.Fatalf("overstated a past limit: %+v", worker)
			}
		})
	}
}

func TestRenderWorkerStatusIdentifiesMissingReadings(t *testing.T) {
	value := float64(0)
	snapshot := workerStatusSnapshot{
		Workers: []workerStatus{{ID: "fixture", State: "working", StateSource: "structured session events", Limit: "unavailable", LimitSource: "no current session limit signal"}},
		Accounts: []llmbudget.AccountReport{{
			Provider: "fixture", Account: "test", Pool: "test-pool", AccountKnown: true,
			Metrics: []llmbudget.Metric{{Name: "billing.spend", Value: &value, Unit: "usd", Classification: "actual", Source: "local-meter"}},
		}},
	}
	var out bytes.Buffer
	if err := renderWorkerStatus(&out, snapshot); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"working", "limit unavailable", "Quota unavailable", "Reset unavailable", `"billing.spend" = 0 "usd"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in output:\n%s", want, out.String())
		}
	}
}
