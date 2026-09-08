//go:build !windows

package agentos

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/room"
)

// Opt-in actual watcher-loop experiment. Configuration is captured before the
// fixture strips inherited BASHY overrides. Each run has pending mail, native
// notifications and unrelated filesystem churn. No real inbox or lease is used.
func TestSprintWatchP0Performance(t *testing.T) {
	if os.Getenv("BASHY_P0_PERF") != "1" {
		t.Skip("set BASHY_P0_PERF=1 for the >=60s watcher experiment")
	}
	mode, history := os.Getenv("BASHY_P0_PERF_MODE"), os.Getenv("BASHY_P0_PERF_HISTORY")
	reportPath := os.Getenv("BASHY_P0_PERF_REPORT")
	duration, err := time.ParseDuration(os.Getenv("BASHY_P0_PERF_DURATION"))
	if err != nil || duration < 60*time.Second {
		t.Fatal("BASHY_P0_PERF_DURATION must be >=60s")
	}
	if mode != "baseline" && mode != "fixed" {
		t.Fatal("BASHY_P0_PERF_MODE must be baseline or fixed")
	}
	if history != "small" && history != "large" {
		t.Fatal("BASHY_P0_PERF_HISTORY must be small or large")
	}
	if !filepath.IsAbs(reportPath) {
		t.Fatal("BASHY_P0_PERF_REPORT must be an absolute output path")
	}
	path := sprintWatchIsolate(t)
	count := 100
	if history == "large" {
		count = 200000
	}
	line, _ := json.Marshal(room.Event{Type: room.EventNote, Actor: "other-agent", Topic: "unrelated", Body: strings.Repeat("x", 160)})
	data := []byte(strings.Repeat(string(line)+"\n", count))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if history == "large" && len(data) < 20_000_000 {
		t.Fatal("large fixture below 20 MB")
	}
	reader := newSprintWatchAckReader()
	rt := defaultSprintWatchRuntime()
	rt.beat = nil
	rt.release = nil
	rt.ackEvery = 10 * time.Second
	rt.poll.snapshot = func(string, int, bool) (inboxBatch, error) {
		return inboxBatch{events: []unifiedInboxEvent{{Schema: unifiedInboxSchema, Source: "mb", Seq: 1, Body: "pending unacknowledged synthetic mail"}}, acks: []func() error{func() error { t.Error("pending mail was consumed without ack"); return nil }}}, nil
	}
	var baselineStats room.TimelineReadStats
	rt.ackSeq = func(id int64, owner string) (int64, error) {
		baselineStats.BytesRead += int64(len(data))
		baselineStats.BytesDecoded += int64(len(data))
		baselineStats.RecordsDecoded += int64(count)
		return latestSprintWatchAckP0Baseline(id, owner)
	}
	rt.ackPosition = reader.latest
	// Warm the selected reader before any allocation/CPU/clock measurement.
	if mode == "fixed" {
		if _, err := reader.latest(context.Background(), 138, "manager"); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := rt.ackSeq(138, "manager"); err != nil {
			t.Fatal(err)
		}
	}
	initialStats := baselineStats
	if mode == "fixed" {
		initialStats = reader.reader.Stats()
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	var cpuBefore, cpuAfter syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &cpuBefore); err != nil {
		t.Fatal(err)
	}
	out := &p0CountOutput{}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	started := time.Now()
	var churn sync.WaitGroup
	churn.Add(1)
	go func() {
		defer churn.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = os.WriteFile(filepath.Join(room.Dir(), "unrelated-churn"), []byte("changed"), 0600)
			}
		}
	}()
	run := runSprintInboxWatch
	if mode == "baseline" {
		run = runSprintInboxWatchP0Baseline
	}
	err = run(ctx, out, out, 138, "manager", rt)
	cancel()
	churn.Wait()
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &cpuAfter); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	stats := baselineStats
	if mode == "fixed" {
		stats = reader.reader.Stats()
	}
	decoded := stats.BytesDecoded - initialStats.BytesDecoded
	if mode == "fixed" && decoded != 0 {
		t.Fatalf("unchanged steady history decoded %d bytes", decoded)
	}
	report := map[string]any{
		"mode": mode, "history": history, "event_count": count, "history_bytes": len(data), "steady_seconds": elapsed.Seconds(),
		"cpu_seconds": p0CPU(cpuAfter) - p0CPU(cpuBefore), "alloc_bytes": after.TotalAlloc - before.TotalAlloc,
		"bytes_read": stats.BytesRead - initialStats.BytesRead, "bytes_decoded": decoded, "records_decoded": stats.RecordsDecoded - initialStats.RecordsDecoded,
		"reminder_count": out.reminders, "output_bytes": out.bytes, "inference_calls": 0,
		"compiler": runtime.Version(), "gomaxprocs": runtime.GOMAXPROCS(0), "churn_interval": "5ms", "poll_interval": "100ms",
		"inference_count_kind": "structural call-path audit; not a measured counter",
		"inference_audit":      "watch loop, reader and filesystem notifier only; fixture snapshot and lease hooks; no model invocation seam",
	}
	// Separately verify bounded work when the same reducer sees appended events.
	if mode == "fixed" {
		prior := reader.reader.Stats()
		sprintWatchWriteEvents(t, path, room.Event{Type: room.EventNote}, room.Event{Type: room.EventAck, Topic: sprintWatchTopic(138), Actor: "manager", Target: room.AgentClaimID("manager")})
		pos, err := reader.latest(context.Background(), 138, "manager")
		if err != nil {
			t.Fatal(err)
		}
		appended := reader.reader.Stats()
		if appended.RecordsDecoded-prior.RecordsDecoded != 2 || pos.Seq != int64(count+2) {
			t.Fatalf("append bound/ack failed: %+v -> %+v position %+v", prior, appended, pos)
		}
		report["append_records_decoded"] = appended.RecordsDecoded - prior.RecordsDecoded
		report["append_bytes_read"] = appended.BytesRead - prior.BytesRead
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("P0_PERF %s", b)
}
func p0CPU(r syscall.Rusage) float64 {
	return float64(r.Utime.Sec+r.Stime.Sec) + float64(r.Utime.Usec+r.Stime.Usec)/1e6
}

type p0CountOutput struct{ bytes, reminders int64 }

func (o *p0CountOutput) Write(p []byte) (int, error) {
	o.bytes += int64(len(p))
	if strings.Contains(string(p), `"type":"unacknowledged-inbox"`) {
		o.reminders++
	}
	return len(p), nil
}
