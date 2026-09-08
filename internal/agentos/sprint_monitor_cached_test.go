package agentos

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/llmbudget"
	"github.com/qiangli/coreutils/pkg/resources"
)

type cachedMonitorQuotaAdapter struct{}

func (cachedMonitorQuotaAdapter) Kind() string { return "cached-monitor-test" }
func (cachedMonitorQuotaAdapter) Collect(_ context.Context, _ llmbudget.SourceConfig, at time.Time) (llmbudget.SourceResult, error) {
	used := 50.0
	return llmbudget.SourceResult{Status: "ok", Metrics: []llmbudget.Metric{{Name: "quota.used_percent", Value: &used, Classification: "actual", Source: "cached monitor fixture", ObservedAt: at}}}, nil
}

func TestSprintMonitorRealCachedCollectionDoesNotRewriteAlerts(t *testing.T) {
	sprintWatchIsolate(t)
	policy := &llmbudget.Policy{Version: 1,
		Bindings: []llmbudget.Binding{{Model: "fixture", Provider: "fixture", Account: "account", Pool: "shared", Lane: llmbudget.LaneSubscription, AccountKnown: true}},
		Sources:  []llmbudget.SourceConfig{{ID: "fixture", Kind: "cached-monitor-test", Provider: "fixture", Account: "account", Pool: "shared", Lane: llmbudget.LaneSubscription, Enabled: true, RefreshSeconds: 60, TimeoutSeconds: 1}},
	}
	t.Cleanup(llmbudget.SetDefault(llmbudget.New(llmbudget.Config{
		StatePath: filepath.Join(t.TempDir(), "meter.json"), Policy: policy,
		Models:   map[string]llmbudget.Model{"fixture": {Name: "fixture", Provider: "fixture", Kind: "subscription", Billing: "flat"}},
		Adapters: []llmbudget.Adapter{cachedMonitorQuotaAdapter{}},
	})))
	ctx := context.Background()
	targets := map[int64]string{138: "owner", 139: "competitor"}
	var previous *sprintMonitorSnapshot
	var revision uint64
	cached := 0
	for i := 0; i < 9; i++ {
		s, err := collectSprintMonitor(ctx, sprintMonitorOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if s.Host == nil || s.Models == nil {
			t.Fatal("real collector omitted source sections")
		}
		if _, err := updateSprintAlertTargets(ctx, s, targets, 0, false); err != nil {
			t.Fatal(err)
		}
		ledger, err := resources.ReadAlertState(ctx, resources.ResourcesStateDir())
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil && s.Host.At.Equal(previous.Host.At) && !s.Host.Sections["cpu"].Stale && !s.Host.Sections["memory"].Stale {
			cached++
			if s.At.Equal(previous.At) || s.Models.GeneratedAt.Equal(previous.Models.GeneratedAt) {
				t.Fatal("real caller did not advance presentation timestamps")
			}
			if ledger.Revision != revision {
				t.Fatalf("cached sources rewrote alert ledger: %d -> %d", revision, ledger.Revision)
			}
		}
		previous, revision = s, ledger.Revision
	}
	if cached == 0 {
		t.Fatal("no cached-source observation was exercised")
	}
}

func BenchmarkSprintMonitorCachedAlertEvaluation(b *testing.B) {
	sprintWatchIsolate(b)
	s := monitorFixture(time.Now().UTC(), 50, false)
	targets := map[int64]string{}
	for id := int64(1); id <= 10; id++ {
		targets[id] = "owner"
	}
	ctx := context.Background()
	if _, err := updateSprintAlertTargets(ctx, s, targets, 0, false); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := updateSprintAlertTargets(ctx, s, targets, 0, true); err != nil {
			b.Fatal(err)
		}
	}
}
