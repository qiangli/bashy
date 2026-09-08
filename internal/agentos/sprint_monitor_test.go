package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/foreman"
	"github.com/qiangli/coreutils/pkg/llmbudget"
	"github.com/qiangli/coreutils/pkg/resources"
	"github.com/qiangli/coreutils/pkg/weave"
)

func monitorFixture(at time.Time, cpu float64, stale bool) *sprintMonitorSnapshot {
	return &sprintMonitorSnapshot{SchemaVersion: sprintMonitorSchema, At: at, Sprint: 138, Host: &resources.HostObservation{At: at, Sections: map[string]resources.ObservationStatus{"cpu": {Kind: "actual", At: at, Stale: stale}, "memory": {Kind: "actual", At: at}}, System: &resources.System{Host: "fixture", CPU: resources.CPU{UsagePercent: cpu, Source: "ticks"}, Memory: resources.Memory{UsedPercent: 50, TotalBytes: 100}}}, Models: &llmbudget.Report{SchemaVersion: llmbudget.ReportSchemaVersion}}
}
func TestSprintMonitorWatchBoundsOutputAndIgnoresTimestampOnlyChanges(t *testing.T) {
	sprintWatchIsolate(t)
	now := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	rt := sprintMonitorRuntime{now: func() time.Time { return now }, collect: func(context.Context, sprintMonitorOptions) (*sprintMonitorSnapshot, error) {
		calls++
		snapshot := monitorFixture(now, 10, false)
		quota := 42.0
		snapshot.Models.Accounts = []llmbudget.AccountReport{{Provider: "fixture", Account: "shared", Status: "ok", Metrics: []llmbudget.Metric{{Name: "quota.used_percent", Value: &quota, Classification: "actual", ObservedAt: now, Source: "fixture"}}}}
		return snapshot, nil
	}, wait: func(context.Context, time.Duration) error {
		now = now.Add(30 * time.Second)
		if calls == 3 {
			cancel()
		}
		return nil
	}}
	var out bytes.Buffer
	if err := runSprintMonitor(ctx, &out, sprintMonitorOptions{Sprint: 138}, true, true, false, 30*time.Second, rt); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("timestamp-only output spam: %s", out.String())
	}
	for i, line := range lines {
		if len(line)+1 > 4096 {
			t.Fatal("output budget exceeded")
		}
		var e sprintMonitorEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		if len(e.Accounts) != 1 || e.Accounts[0].Metrics[0].ObservedAt.IsZero() {
			t.Fatal("emitted known quota lost actual observation timestamp")
		}
		want := "snapshot"
		if i == 1 {
			want = "heartbeat"
		}
		if e.Type != want {
			t.Fatalf("event=%s want %s", e.Type, want)
		}
	}
}
func TestSprintMonitorOverflowKeepsPendingCountAndDrilldown(t *testing.T) {
	e := sprintMonitorEvent{SchemaVersion: sprintMonitorSchema, Summary: summarizeSprintMonitor(monitorFixture(time.Now(), 95, false))}
	for i := 0; i < 30; i++ {
		e.Alerts = append(e.Alerts, sprintResourceAlert{ID: strings.Repeat("x", 64), Message: strings.Repeat("m", 512)})
	}
	b, err := boundedMonitorEvent(e)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > 4095 {
		t.Fatal("oversized record")
	}
	var result sprintMonitorEvent
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if result.Pending == 0 || result.Summary.Read == "" || len(result.Alerts) == 0 {
		t.Fatalf("overflow lost actionable summary: %+v", result)
	}
}
func TestSprintMonitorAlertsSustainRecoverAndSurviveHandoff(t *testing.T) {
	sprintWatchIsolate(t)
	at := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	update := func(seconds int, cpu float64, stale bool, owner string) []sprintResourceAlert {
		t.Helper()
		a, e := updateSprintAlerts(context.Background(), monitorFixture(at.Add(time.Duration(seconds)*time.Second), cpu, stale), 138, owner, false)
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	if len(update(0, 95, false, "first")) != 0 || len(update(10, 95, false, "first")) != 0 {
		t.Fatal("pressure alerted without sustained samples")
	}
	if len(update(10, 95, false, "first")) != 0 {
		t.Fatal("same sample advanced onset")
	}
	a := update(15, 95, false, "first")
	if len(a) != 1 {
		t.Fatalf("onset=%+v", a)
	}
	id := a[0].ID
	a = update(15, 95, false, "second")
	if len(a) != 1 || a[0].ID == id || a[0].Owner != "second" {
		t.Fatal("handoff lost active condition")
	}
	if len(update(50, 0, true, "second")) != 1 {
		t.Fatal("stale zero reported recovery")
	}
	if len(update(60, 0, false, "second")) != 1 || len(update(89, 0, false, "second")) != 1 {
		t.Fatal("recovery window shortened")
	}
	if len(update(90, 0, false, "second")) != 0 {
		t.Fatal("sustained recovery not recognized")
	}
	ledger, e := resources.ReadAlertState(context.Background(), resources.ResourcesStateDir())
	if e != nil {
		t.Fatal(e)
	}
	recovered := false
	for _, raw := range ledger.Entries {
		var c sprintAlertCondition
		_ = json.Unmarshal(raw, &c)
		for _, p := range c.Pending {
			if p.Severity == "recovered" && p.Owner == "second" {
				recovered = true
			}
		}
	}
	if !recovered {
		t.Fatal("recovery not durable for successor")
	}
}
func TestSprintMonitorAlertPublicationReplaysWithoutConsumingInbox(t *testing.T) {
	sprintWatchIsolate(t)
	at := time.Now().UTC()
	board := map[string]any{"stories": []any{map[string]any{"id": 138, "owner": "manager", "column": "doing", "boxes": []any{map[string]any{"started_at": at, "cutoff": at.Add(time.Hour)}}}}}
	b, _ := json.Marshal(board)
	if err := os.WriteFile(filepath.Join(os.Getenv("BASHY_SPRINT_DIR"), "queue.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	for _, seconds := range []int{0, 15, 15} {
		if _, err := updateSprintAlerts(context.Background(), monitorFixture(at.Add(time.Duration(seconds)*time.Second), 95, false), 138, "manager", true); err != nil {
			t.Fatal(err)
		}
	}
	posts, err := bus.Posts()
	if err != nil || len(posts) != 1 {
		t.Fatalf("notice duplicated: %+v %v", posts, err)
	}
	if posts[0].To != "manager" || posts[0].IdempotencyKey == "" {
		t.Fatal("unaddressed notice")
	}
	if bus.SeenSeq("manager") != 0 {
		t.Fatal("observation consumed manager inbox")
	}
}
func TestSprintWatchObservationNeverBlocksUnreadReminderAndJoins(t *testing.T) {
	sprintWatchIsolate(t)
	var calls atomic.Int64
	var stopped atomic.Bool
	var released atomic.Bool
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	rt := sprintWatchRuntime{ackEvery: 5 * time.Millisecond, ackSeq: func(int64, string) (int64, error) { return 0, nil }, poll: sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) {
		return inboxBatch{events: []unifiedInboxEvent{{Body: "pending"}}}, nil
	}), observeEvery: time.Millisecond, observe: func(ctx context.Context, _ int64, _ string) { calls.Add(1); <-ctx.Done(); stopped.Store(true) }, release: func(int64, string) error { released.Store(true); return nil }}
	var out bytes.Buffer
	if err := runSprintInboxWatch(ctx, &out, &out, 138, "manager", rt); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || !stopped.Load() || !released.Load() {
		t.Fatal("observer lifetime/lease cleanup incorrect")
	}
	if strings.Count(out.String(), "unacknowledged-inbox") < 4 {
		t.Fatal("slow observer blocked reminders")
	}
}
func TestSprintMonitorNativeHookIsScopedAndCancellable(t *testing.T) {
	if foreman.ObserveSession == nil {
		t.Fatal("native observation hook missing")
	}
	if stop := foreman.ObserveSession(context.Background(), "unrelated-session", "manager"); stop != nil {
		stop()
		t.Fatal("generic session acquired sprint observer")
	}
	stop := foreman.ObserveSession(context.Background(), "sprint-138-manager", "manager")
	if stop == nil {
		t.Fatal("native sprint session has no observer")
	}
	stop()
}
func TestModelsResourceCommandsShareReportWithoutAdmission(t *testing.T) {
	sprintWatchIsolate(t)
	value := float64(50)
	report := &llmbudget.Report{SchemaVersion: llmbudget.ReportSchemaVersion, Accounts: []llmbudget.AccountReport{{Provider: "vendor", Account: "shared", Pool: "pool", Models: []string{"selected", "competitor"}, Agents: []string{"one", "two"}, Metrics: []llmbudget.Metric{{Name: "usage.input_tokens", Value: &value}, {Name: "quota.used_percent", Value: &value}, {Name: "budget.daily_tokens", Value: &value}}}}}
	for _, verb := range []string{"usage", "limits", "budget"} {
		calls := 0
		cmd := modelResourcesCommand(verb, func(_ context.Context, opt llmbudget.ReportOptions) (*llmbudget.Report, error) {
			calls++
			if opt.Model != "selected" {
				t.Fatal("filter not passed")
			}
			return report, nil
		})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--model", "selected", "--json"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var got llmbudget.Report
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if calls != 1 || len(got.Accounts[0].Models) != 2 || len(got.Accounts[0].Metrics) == 0 {
			t.Fatalf("shared account hidden: %+v", got)
		}
		for _, m := range got.Accounts[0].Metrics {
			if !modelMetricVisible(verb, m.Name) {
				t.Fatal("wrong metric view")
			}
		}
	}
	if len(report.Accounts[0].Metrics) != 3 {
		t.Fatal("command mutated shared report cache")
	}
}

func TestModelsResourcePolicyConfigureRequiresExplicitApply(t *testing.T) {
	for _, apply := range []bool{false, true} {
		called := false
		cmd := modelBudgetConfigureCommand(func(ctx context.Context, path string, got bool) (llmbudget.PolicyChange, error) {
			called = true
			if path != "fixture.json" || got != apply {
				t.Fatalf("wrong mutation request %q %v", path, got)
			}
			return llmbudget.PolicyChange{Path: "policy.json", Applied: got, Version: 1}, nil
		})
		args := []string{"--file", "fixture.json", "--json"}
		if apply {
			args = append(args, "--apply")
		}
		cmd.SetArgs(args)
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Fatal("configure not called")
		}
		var result llmbudget.PolicyChange
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Applied != apply {
			t.Fatal("dry run mutated policy")
		}
	}
}
func TestSprintMonitorSharedSnapshotDoesNotRewriteAlertLedger(t *testing.T) {
	sprintWatchIsolate(t)
	s := monitorFixture(time.Now().UTC(), 50, false)
	targets := map[int64]string{138: "owner", 139: "competitor"}
	if _, err := updateSprintAlertTargets(context.Background(), s, targets, 0, false); err != nil {
		t.Fatal(err)
	}
	first, err := resources.ReadAlertState(context.Background(), resources.ResourcesStateDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		s.At = s.At.Add(time.Millisecond)
		if _, err := updateSprintAlertTargets(context.Background(), s, targets, 0, false); err != nil {
			t.Fatal(err)
		}
	}
	last, err := resources.ReadAlertState(context.Background(), resources.ResourcesStateDir())
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != last.Revision || !first.UpdatedAt.Equal(last.UpdatedAt) {
		t.Fatalf("shared snapshot caused writes: %d -> %d", first.Revision, last.Revision)
	}
}
func TestSprintMonitorQuotaWindowsRemainDistinct(t *testing.T) {
	s := monitorFixture(time.Now(), 50, false)
	v := 95.0
	s.Models.Accounts = []llmbudget.AccountReport{{Provider: "claude", Account: "same", AccountKnown: true, Pool: "shared", Metrics: []llmbudget.Metric{{Name: "quota.used_percent", Value: &v, Classification: "actual", Source: "claude-statusline:five_hour"}, {Name: "quota.used_percent", Value: &v, Classification: "actual", Source: "claude-statusline:seven_day"}}}}
	samples := sprintAlertSamples(s)
	seen := map[string]bool{}
	for _, sample := range samples {
		if seen[sample.key] {
			t.Fatal("shared quota windows collapsed")
		}
		seen[sample.key] = true
	}
}

func TestSprintMonitorRemoteSnapshotPreservesInventoryWithoutLocalNotices(t *testing.T) {
	sprintWatchIsolate(t)
	now := time.Now().UTC()
	s := monitorFixture(now, 95, false)
	s.Inventory = &weave.SprintInventory{At: now, Complete: true, Sprints: []weave.SprintInventorySeat{{ID: 138, Owner: "remote-owner", Active: true}}}
	raw, _ := json.Marshal(s)
	remote, e := decodeRemoteSprintMonitor(raw, sprintMonitorOptions{Sprint: 138, Host: "fixture-remote"}, now)
	if e != nil || remote.Origin != "fixture-remote" || remote.Inventory == nil {
		t.Fatalf("remote attribution lost: %+v %v", remote, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	rt := sprintMonitorRuntime{now: func() time.Time { return now }, collect: func(context.Context, sprintMonitorOptions) (*sprintMonitorSnapshot, error) {
		calls++
		return remote, nil
	}, wait: func(context.Context, time.Duration) error {
		now = now.Add(20 * time.Second)
		if calls == 2 {
			cancel()
		}
		return nil
	}}
	var out bytes.Buffer
	if e = runSprintMonitor(ctx, &out, sprintMonitorOptions{Sprint: 138, Host: "fixture-remote"}, true, true, false, 5*time.Second, rt); e != nil {
		t.Fatal(e)
	}
	ledger, e := resources.ReadAlertState(context.Background(), resources.ResourcesStateDir())
	if e != nil || len(ledger.Entries) != 0 {
		t.Fatal("remote observation evaluated local owner alerts")
	}
	s.At = now.Add(time.Minute)
	raw, _ = json.Marshal(s)
	if _, e = decodeRemoteSprintMonitor(raw, sprintMonitorOptions{}, now); e == nil {
		t.Fatal("future remote data accepted")
	}
}
