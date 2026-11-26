package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/llmbudget"
)

func TestWorkerStatusWatchEmitsOnlyChangedObservations(t *testing.T) {
	at := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	samples := []workerStatusSnapshot{
		{At: at, Workers: []workerStatus{{ID: "worker-b", Task: "setting up"}, {ID: "worker-a"}}},
		{At: at.Add(5 * time.Second), Workers: []workerStatus{{ID: "worker-a"}, {ID: "worker-b", Task: "running the check"}}},
		{At: at.Add(10 * time.Second), Workers: []workerStatus{{ID: "worker-b", Task: "running the check"}, {ID: "worker-a"}}},
	}
	collected := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt := workerStatusRuntime{
		collect: func(context.Context) (workerStatusSnapshot, error) {
			sample := samples[collected]
			collected++
			return sample, nil
		},
		wait: func(_ context.Context, cadence time.Duration) error {
			if cadence != 5*time.Second {
				t.Fatalf("watch cadence = %v, want 5s", cadence)
			}
			if collected == len(samples) {
				cancel()
			}
			return nil
		},
	}
	var out bytes.Buffer
	cmd := workerStatusCommand(rt)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--watch", "30s", "--diff", "--json"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&out)
	var snapshot, change, unexpected workerStatusWatchEvent
	if err := decoder.Decode(&snapshot); err != nil || snapshot.Type != "snapshot" || snapshot.Snapshot == nil {
		t.Fatalf("initial snapshot = %+v, err = %v", snapshot, err)
	}
	if err := decoder.Decode(&change); err != nil || change.Type != "change" || len(change.Changes) != 1 {
		t.Fatalf("change event = %+v, err = %v", change, err)
	}
	got := change.Changes[0]
	if got.Entity != "worker" || len(got.Key) != 1 || got.Key[0] != "worker-b" ||
		got.Field != "task" || got.Before != "setting up" || got.After != "running the check" {
		t.Fatalf("changed field = %+v", got)
	}
	if err := decoder.Decode(&unexpected); err != io.EOF {
		t.Fatalf("unchanged observation emitted data: %+v, err = %v", unexpected, err)
	}
}

func TestWorkerStatusWatchUsesDurationDeadline(t *testing.T) {
	collected := 0
	rt := workerStatusRuntime{
		collect: func(context.Context) (workerStatusSnapshot, error) {
			collected++
			return workerStatusSnapshot{}, nil
		},
		wait: func(ctx context.Context, _ time.Duration) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("watch did not set a deadline")
			}
			<-ctx.Done()
			if ctx.Err() != context.DeadlineExceeded {
				t.Fatalf("watch context ended with %v", ctx.Err())
			}
			return ctx.Err()
		},
	}
	var out bytes.Buffer
	cmd := workerStatusCommand(rt)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--watch", "50ms", "--json"})
	if err := cmd.Execute(); err != nil || collected != 1 {
		t.Fatalf("watch collected %d snapshots before exit, err = %v", collected, err)
	}
}

func TestWorkerStatusWatchRejectsInvalidFlags(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--watch", "0s"}, "positive duration"},
		{[]string{"--watch", "-1s"}, "positive duration"},
		{[]string{"--diff"}, "requires --watch"},
	} {
		cmd := workerStatusCommand(workerStatusRuntime{})
		cmd.SetArgs(tc.args)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v returned %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestWorkerStatusDiffKeepsDistinctAndRepeatedAccountMetrics(t *testing.T) {
	at := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	reset := at.Add(time.Hour)
	laterReset := reset.Add(time.Hour)
	low, high := float64(50), float64(80)
	metric := llmbudget.Metric{
		Name: "quota.used_percent", Source: "cached", Unit: "percent",
		Window: "5h", Value: &low, Classification: "actual", ObservedAt: at, ResetAt: &reset,
	}
	repeat := metric
	repeat.Value = &high
	weekly := metric
	weekly.Window = "week"
	account := llmbudget.AccountReport{
		Provider: "demo", Account: "example", Pool: "subscription", Lane: llmbudget.LaneSubscription,
		Agents: []string{"beta", "alpha"}, Metrics: []llmbudget.Metric{metric, repeat, weekly},
	}
	before := workerStatusSnapshot{Accounts: []llmbudget.AccountReport{account}, Warnings: []string{"b", "a"}}
	updated := account
	updated.Agents = []string{"alpha", "beta"}
	updated.Metrics = []llmbudget.Metric{weekly, repeat, metric}
	for i := range updated.Metrics {
		updated.Metrics[i].ObservedAt = at.Add(time.Minute)
	}
	after := workerStatusSnapshot{At: at.Add(time.Minute), Accounts: []llmbudget.AccountReport{updated}, Warnings: []string{"a", "b"}}
	if changes := diffWorkerStatus(before, after); len(changes) != 0 {
		t.Fatalf("timestamp or ordering created a diff: %+v", changes)
	}

	after.Accounts[0].Metrics[1].ResetAt = &laterReset
	changes := diffWorkerStatus(before, after)
	if len(changes) != 1 || changes[0].Entity != "account" || changes[0].Metric == nil || changes[0].Metric.Window != "5h" {
		t.Fatalf("changed metric = %+v", changes)
	}
	beforeReadings := changes[0].Before.([]workerStatusMetricReading)
	afterReadings := changes[0].After.([]workerStatusMetricReading)
	if len(beforeReadings) != 2 || len(afterReadings) != 2 || !afterReadings[1].ResetAt.Equal(laterReset) {
		t.Fatalf("repeated readings lost: before=%+v after=%+v", beforeReadings, afterReadings)
	}
}

func TestWorkerStatusDiffReportsAddedAndRemovedRecords(t *testing.T) {
	before := workerStatusSnapshot{Workers: []workerStatus{{ID: "departed"}}}
	after := workerStatusSnapshot{Workers: []workerStatus{{ID: "arrived"}}}
	changes := diffWorkerStatus(before, after)
	if len(changes) != 2 || changes[0].Field != "record" || changes[0].Before != nil ||
		changes[0].After.(workerStatus).ID != "arrived" || changes[1].Field != "record" ||
		changes[1].Before.(workerStatus).ID != "departed" || changes[1].After != nil {
		t.Fatalf("join/leave changes = %+v", changes)
	}
}
