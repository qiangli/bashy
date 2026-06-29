// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"path/filepath"
	"strings"
	"testing"
)

// newTestMemory builds an in-memory store with persistence pointed at a temp
// file and a fixed clock, for deterministic assertions.
func newTestMemory(t *testing.T) *memory {
	t.Helper()
	return &memory{
		path:  filepath.Join(t.TempDir(), "hosts.json"),
		nowFn: func() int64 { return 1000 },
		hosts: map[string]hostRecord{},
		fails: map[string]int{},
	}
}

func TestMemoryRecordFailAndClear(t *testing.T) {
	m := newTestMemory(t)
	k := loopKey([]string{"ssh", "host.local"})
	if n := m.recordFail(k); n != 1 {
		t.Fatalf("first fail = %d, want 1", n)
	}
	if n := m.recordFail(k); n != 2 {
		t.Fatalf("second fail = %d, want 2", n)
	}
	m.clearFail(k)
	if n := m.recordFail(k); n != 1 {
		t.Fatalf("after clear, fail = %d, want 1", n)
	}
}

func TestMemoryPersistRoundTrip(t *testing.T) {
	m := newTestMemory(t)
	m.recordSuccess("host.local", "fpA") // new host ⇒ persisted

	// A fresh memory at the same path must see the recorded host.
	m2 := &memory{path: m.path, nowFn: m.nowFn, hosts: map[string]hostRecord{}, fails: map[string]int{}}
	m2.load()
	rec, ok := m2.priorSuccess("host.local")
	if !ok {
		t.Fatal("expected persisted host.local after reload")
	}
	if rec.NetFP != "fpA" || rec.Successes != 1 {
		t.Errorf("reloaded record = %+v, want NetFP=fpA Successes=1", rec)
	}
}

func TestMemoryPersistMergesConcurrentWriters(t *testing.T) {
	// Two memories share a path; each records a distinct host. After both
	// persist, the file must contain both (merge, not clobber).
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.json")
	mk := func() *memory {
		return &memory{path: path, nowFn: func() int64 { return 1000 }, hosts: map[string]hostRecord{}, fails: map[string]int{}}
	}
	a, b := mk(), mk()
	a.recordSuccess("alpha.local", "fpA")
	b.recordSuccess("beta.local", "fpB")

	got := readMemoryFile(path)
	if _, ok := got["alpha.local"]; !ok {
		t.Error("merged ledger missing alpha.local")
	}
	if _, ok := got["beta.local"]; !ok {
		t.Error("merged ledger missing beta.local")
	}
}

func TestCapHostRecords(t *testing.T) {
	hosts := map[string]hostRecord{
		"a": {LastSuccess: 1}, "b": {LastSuccess: 2}, "c": {LastSuccess: 3},
	}
	capHostRecords(hosts, 2)
	if len(hosts) != 2 {
		t.Fatalf("len = %d, want 2", len(hosts))
	}
	if _, ok := hosts["a"]; ok { // oldest dropped
		t.Error("expected the oldest (a) to be evicted")
	}
}

func TestApplyLoopBelowThreshold(t *testing.T) {
	base := &hint{dimension: "disk", retryable: true, text: "x", suggest: "y"}
	if got := applyLoop(base, "cp", 2); got != base || got.retryable != true {
		t.Error("below threshold must return the base hint unchanged")
	}
}

func TestApplyLoopEscalatesBaseHint(t *testing.T) {
	base := &hint{dimension: "network", retryable: false, text: "x", suggest: "use the tunnel"}
	got := applyLoop(base, "ssh", loopThreshold)
	if got.retryable {
		t.Error("escalated hint must be retryable=false")
	}
	if !strings.Contains(got.suggest, "repeatedly") {
		t.Errorf("escalated suggest should note repetition: %q", got.suggest)
	}
}

func TestApplyLoopStandaloneWhenNoBase(t *testing.T) {
	got := applyLoop(nil, "make", loopThreshold+2)
	if got == nil || got.dimension != "loop" {
		t.Fatalf("expected a standalone loop hint, got %+v", got)
	}
	if got.retryable {
		t.Error("loop hint must be retryable=false")
	}
}

func TestNetworkFingerprintStable(t *testing.T) {
	// Whatever the host's real network, the fingerprint must be deterministic
	// within a run (same value on repeated calls) — that stability is what lets
	// "same network as before?" comparisons work.
	a := networkFingerprint()
	b := networkFingerprint()
	if a != b {
		t.Errorf("fingerprint not stable: %q vs %q", a, b)
	}
}

func TestAdviseNetworkHistoryDifferentNetwork(t *testing.T) {
	// host.local was reached before under fingerprint "home"; now we're on
	// "cafe" and it won't resolve ⇒ the stronger, history-grounded hint.
	m := newTestMemory(t)
	m.hosts["host.local"] = hostRecord{NetFP: "home", LastSuccess: 1000, Successes: 3}
	p := healthyProbe()
	p.resolveHost = func(string) bool { return false }
	a := &advisor{probe: p, mem: m, netfp: "cafe"}

	h := a.advise("/repo", []string{"ssh", "host.local"}, 255)
	if h == nil || h.dimension != "network" {
		t.Fatalf("expected network hint, got %+v", h)
	}
	if !strings.Contains(h.text, "moved off its network") {
		t.Errorf("expected the history-grounded wording, got %q", h.text)
	}
}

func TestAdviseNetworkHistorySameNetworkFallsBack(t *testing.T) {
	// Prior success under the SAME fingerprint ⇒ no "moved" claim; the generic
	// lanish hint is used instead.
	m := newTestMemory(t)
	m.hosts["host.local"] = hostRecord{NetFP: "home", LastSuccess: 1000, Successes: 3}
	p := healthyProbe()
	p.resolveHost = func(string) bool { return false }
	a := &advisor{probe: p, mem: m, netfp: "home"}

	h := a.advise("/repo", []string{"ssh", "host.local"}, 255)
	if h == nil || h.dimension != "network" {
		t.Fatalf("expected network hint, got %+v", h)
	}
	if strings.Contains(h.text, "moved off its network") {
		t.Errorf("same-network must not claim a move: %q", h.text)
	}
}
