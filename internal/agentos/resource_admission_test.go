package agentos

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/resources"
	"github.com/qiangli/coreutils/pkg/weave"
)

func TestHostAdmissionPressureUnknownAndFilesystem(t *testing.T) {
	now := time.Now()
	cpu := 90.0
	memory := uint64(100)
	disk := uint64(100)
	// Use absolute paths on the fixture's volume, including a drive letter on
	// Windows. A bare slash is not an absolute Windows filesystem observation.
	root := t.TempDir()
	queue := filepath.Join(root, "queue")
	p := hostAdmissionPolicy{Version: 1, MaxCPUPercent: &cpu, MinAvailableMemoryBytes: &memory, MinFreeDiskBytes: &disk}
	obs := &resources.HostObservation{System: &resources.System{CPU: resources.CPU{Source: "ticks", UsagePercent: 20}, Memory: resources.Memory{AvailableBytes: 1000}, Disks: []resources.Disk{{Mount: root, FreeBytes: 1000}, {Mount: queue, FreeBytes: 50}}}, Sections: map[string]resources.ObservationStatus{}}
	for _, s := range []string{"cpu", "memory", "disks"} {
		obs.Sections[s] = resources.ObservationStatus{Kind: "actual", At: now, ExpiresAt: now.Add(time.Second)}
	}
	demand := weave.WeaveResourceDemand{Queue: filepath.Join(queue, "work"), MemoryBytes: 10}
	if err := checkHostAdmission(p, obs, demand, now); err == nil || !strings.Contains(err.Error(), "destination disk free bytes below") {
		t.Fatalf("expected destination disk pressure instead of parent mount: %v", err)
	}
	obs.System.Disks[1].FreeBytes = 1000
	if err := checkHostAdmission(p, obs, demand, now); err != nil {
		t.Fatal(err)
	}
	obs.System.CPU.UsagePercent = 95
	if err := checkHostAdmission(p, obs, demand, now); err == nil {
		t.Fatal("CPU pressure admitted")
	}
	obs.System.CPU.UsagePercent = 20
	s := obs.Sections["cpu"]
	s.Kind = "estimated"
	obs.Sections["cpu"] = s
	if err := checkHostAdmission(p, obs, demand, now); err == nil {
		t.Fatal("estimated CPU treated as actual")
	}
	p.AllowEstimatedCPU = true
	if err := checkHostAdmission(p, obs, demand, now); err != nil {
		t.Fatal(err)
	}
	s.Stale = true
	obs.Sections["cpu"] = s
	if err := checkHostAdmission(p, obs, demand, now); err == nil {
		t.Fatal("stale CPU admitted")
	}
	s.Stale = false
	s.Kind = "actual"
	obs.Sections["cpu"] = s
	unknown := demand
	unknown.MemoryBytes = 0
	if err := checkHostAdmission(p, obs, unknown, now); err == nil {
		t.Fatal("unknown run memory admitted under hard floor")
	}
	obs.System.Memory.AvailableBytes = 105
	if err := checkHostAdmission(p, obs, demand, now); err == nil {
		t.Fatal("run demand omitted from memory floor")
	}
}
