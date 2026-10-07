package agentos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/dag"
	"github.com/qiangli/yoke/pkg/resources"
)

// probeDestDisk runs the capacity probe for worker against a fake policy and disk
// table, returning disk_bytes and whether it was reported as known.
func probeDestDisk(t *testing.T, worker string, policy *dag.CapacityPolicy, policyErr error, disks []resources.Disk) (uint64, bool) {
	t.Helper()
	oldObserve, oldPolicy := capacityObserveHost, capacityLoadPolicy
	t.Cleanup(func() { capacityObserveHost, capacityLoadPolicy = oldObserve, oldPolicy })
	capacityLoadPolicy = func() (*dag.CapacityPolicy, error) { return policy, policyErr }
	now := time.Now()
	actual := resources.ObservationStatus{Kind: "actual", At: now, ExpiresAt: now.Add(time.Minute)}
	capacityObserveHost = func(context.Context, resources.HostObserveOptions) (*resources.HostObservation, error) {
		return &resources.HostObservation{At: now, ExpiresAt: now.Add(time.Minute),
			Sections: map[string]resources.ObservationStatus{"cpu": actual, "memory": actual, "disks": actual},
			System:   &resources.System{OS: runtime.GOOS, Arch: runtime.GOARCH, CPU: resources.CPU{LogicalCores: 8}, Memory: resources.Memory{TotalBytes: 16 << 30, AvailableBytes: 8 << 30}, Disks: disks}}, nil
	}
	obs, err := sprintCapacityServices().Probe(context.Background(), worker)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	v, ok := obs.Facts.Capacities["disk_bytes"]
	return v, ok
}

// realDir returns a symlink-resolved temp dir (macOS /var -> /private/var).
func realDir(t *testing.T, name string) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := filepath.EvalSymlinks(d)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func onePolicy(worker, workspace string) *dag.CapacityPolicy {
	return &dag.CapacityPolicy{Version: 1, LocalWorker: worker, Targets: []dag.CapacityTarget{{Name: "t1", Worker: worker, Workspace: workspace}}}
}

func TestCapacityProbeUnrelatedTinyDiskDoesNotCap(t *testing.T) {
	ws := realDir(t, "ws")
	got, ok := probeDestDisk(t, "w1", onePolicy("w1", ws), nil, []resources.Disk{
		{Mount: filepath.VolumeName(ws) + string(filepath.Separator), FreeBytes: 57 << 30},
		{Mount: "/System/Volumes/xarts", FreeBytes: 481 << 20},
	})
	if !ok || got != 57<<30 {
		t.Fatalf("disk_bytes = %d,%v; want %d,true", got, ok, uint64(57<<30))
	}
}

func TestCapacityProbeFullTargetIsKnownZero(t *testing.T) {
	ws := realDir(t, "ws")
	got, ok := probeDestDisk(t, "w1", onePolicy("w1", ws), nil, []resources.Disk{
		{Mount: filepath.VolumeName(ws) + string(filepath.Separator), FreeBytes: 500 << 30},
		{Mount: ws, FreeBytes: 0},
	})
	if !ok || got != 0 {
		t.Fatalf("disk_bytes = %d,%v; want 0,true (full target is known, not unknown)", got, ok)
	}
}

func TestCapacityProbeUnknownWithoutMatchingPolicy(t *testing.T) {
	ws := realDir(t, "ws")
	disks := []resources.Disk{{Mount: filepath.VolumeName(ws) + string(filepath.Separator), FreeBytes: 100 << 30}}
	t.Setenv("BASHY_WORKSPACE", ws)
	t.Setenv("WEAVE_WORKSPACE", ws)
	cases := map[string]struct {
		worker string
		policy *dag.CapacityPolicy
		err    error
	}{
		"policy error":       {"w1", nil, errors.New("remote capacity policy unavailable")},
		"nil policy":         {"w1", nil, nil},
		"unmatched worker":   {"w2", onePolicy("w2-other", ws), nil},
		"local worker only":  {"w1", &dag.CapacityPolicy{Version: 1, LocalWorker: "w1", Targets: []dag.CapacityTarget{{Name: "t2", Worker: "w9", Workspace: ws}}}, nil},
		"worker as path":     {ws, onePolicy("w1", ws), nil},
		"target name not id": {"t1", onePolicy("w1", ws), nil},
		"empty worker":       {"", onePolicy("w1", ws), nil},
		"no workspace":       {"w1", onePolicy("w1", ""), nil},
		"relative workspace": {"w1", onePolicy("w1", "rel/ws"), nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got, ok := probeDestDisk(t, c.worker, c.policy, c.err, disks); ok {
				t.Fatalf("disk_bytes = %d; want unknown", got)
			}
		})
	}
}

func TestCapacityProbePartialDestinationFailureIsUnknown(t *testing.T) {
	ws := realDir(t, "ws")
	t.Run("missing workspace", func(t *testing.T) {
		if got, ok := probeDestDisk(t, "w1", onePolicy("w1", filepath.Join(ws, "absent")), nil, []resources.Disk{{Mount: filepath.VolumeName(ws) + string(filepath.Separator), FreeBytes: 9 << 30}}); ok {
			t.Fatalf("disk_bytes = %d; want unknown", got)
		}
	})
	t.Run("one valid and one missing destination", func(t *testing.T) {
		policy := onePolicy("w1", ws)
		policy.Targets = append(policy.Targets, dag.CapacityTarget{Name: "t2", Worker: "w1", Workspace: filepath.Join(ws, "missing")})
		if got, ok := probeDestDisk(t, "w1", policy, nil, []resources.Disk{{Mount: filepath.VolumeName(ws) + string(filepath.Separator), FreeBytes: 9 << 30}}); ok {
			t.Fatalf("disk_bytes = %d; want unknown when one destination fails", got)
		}
	})
	t.Run("no covering mount", func(t *testing.T) {
		if got, ok := probeDestDisk(t, "w1", onePolicy("w1", ws), nil, []resources.Disk{{Mount: "/nonexistent-mount", FreeBytes: 9 << 30}}); ok {
			t.Fatalf("disk_bytes = %d; want unknown", got)
		}
	})
}

func TestCapacityProbeResolvesCrossMountSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
	base := realDir(t, "base")
	ws := base
	big := filepath.Join(base, "big")
	small := filepath.Join(base, "small")
	for _, d := range []string{big, small} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(small, "ws")
	if err := os.Symlink(big, link); err != nil {
		t.Fatal(err)
	}
	// The workspace path lexically sits under the small mount but really lives
	// on the big one; only the resolved path may select the mount.
	got, ok := probeDestDisk(t, "w1", onePolicy("w1", link), nil, []resources.Disk{
		{Mount: filepath.VolumeName(ws) + string(filepath.Separator), FreeBytes: 1 << 30},
		{Mount: big, FreeBytes: 200 << 30},
		{Mount: small, FreeBytes: 1 << 20},
	})
	if !ok || got != 200<<30 {
		t.Fatalf("disk_bytes = %d,%v; want %d,true", got, ok, uint64(200<<30))
	}
}

func TestMountCovers(t *testing.T) {
	cases := []struct {
		mount, path string
		want        bool
	}{
		{"/", "/workspace", true},
		{"/", "/", true},
		{"/workspace", "/workspace/sub", true},
		{"/workspace", "/workspaces", false},
		{"/System/Volumes/xarts", "/workspace", false},
		{"", "/workspace", false},
		{"/workspace", "", false},
		{`C:\`, `C:\workspace`, true},
		{`C:\workspace`, `c:\workspace\sub`, true},
		{`C:\workspace`, `D:\workspace`, false},
	}
	for _, c := range cases {
		if got := mountCovers(c.mount, c.path); got != c.want {
			t.Errorf("mountCovers(%q, %q) = %v, want %v", c.mount, c.path, got, c.want)
		}
	}
}
