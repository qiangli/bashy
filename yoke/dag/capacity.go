package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/dag"
	"github.com/qiangli/yoke/pkg/resources"
)

var (
	capacityCollectMonitor = collectSprintMonitor
	capacityObserveHost    = resources.ObserveHost
	capacityLoadPolicy     = dag.LoadCapacityPolicy
)

func sprintCapacityServices() dag.CapacityServices {
	return dag.CapacityServices{
		Identity: func(ctx context.Context, pid int) (string, error) {
			id, e := resources.LookupProcessIdentity(ctx, pid)
			return id.StartID, e
		},
		Probe: func(ctx context.Context, worker string) (dag.CapacityObservation, error) {
			h, err := capacityObserveHost(ctx, resources.HostObserveOptions{})
			if err != nil {
				return dag.CapacityObservation{}, err
			}
			if h == nil || h.System == nil {
				return dag.CapacityObservation{}, errors.New("host observation unavailable")
			}
			sys := h.System
			cpu, mem := h.Sections["cpu"], h.Sections["memory"]
			known := cpu.Kind == "actual" && !cpu.Stale && mem.Kind == "actual" && !mem.Stale && !h.At.After(time.Now()) && time.Now().Before(h.ExpiresAt)
			facts := dag.HostFacts{SchemaVersion: dag.HostFactsSchemaVersion, Worker: worker, OS: sys.OS, Arch: sys.Arch, CPU: sys.CPU.LogicalCores, MemBytes: sys.Memory.TotalBytes, Venues: []string{dag.VenueUserland}, ObservedAt: h.At, Capacities: map[string]uint64{}}
			if st := h.Sections["disks"]; st.Kind == "actual" && !st.Stale {
				if free, ok := selectDestinationDiskFree(worker, sys.Disks); ok {
					facts.Capacities["disk_bytes"] = free
				}
			}
			return dag.CapacityObservation{Facts: facts, HeadroomKnown: known, FreeCPU: math.Max(0, float64(sys.CPU.LogicalCores)*(1-sys.CPU.UsagePercent/100)), FreeMemory: sys.Memory.AvailableBytes}, nil
		},
		Observe: func(ctx context.Context) (json.RawMessage, error) {
			snapshot, e := capacityCollectMonitor(ctx, sprintMonitorOptions{})
			if e != nil {
				return nil, e
			}
			return json.Marshal(snapshot)
		},
	}
}

// selectDestinationDiskFree reports the free bytes of the mount that actually
// holds the worker's configured workspace. The worker is an opaque logical id
// matched exactly against policy targets (dag semantics); a missing policy, an
// unmatched worker, or any unresolvable destination yields unknown.
func selectDestinationDiskFree(worker string, disks []resources.Disk) (uint64, bool) {
	p, err := capacityLoadPolicy()
	if err != nil || p == nil {
		return 0, false
	}
	var minFree uint64
	found := false
	for _, t := range p.Targets {
		if t.Worker != worker || worker == "" {
			continue
		}
		if t.Workspace == "" || !filepath.IsAbs(t.Workspace) {
			return 0, false
		}
		real, err := filepath.EvalSymlinks(t.Workspace)
		if err != nil {
			return 0, false
		}
		d, ok := coveringMount(disks, real)
		if !ok {
			return 0, false
		}
		if !found || d.FreeBytes < minFree {
			minFree, found = d.FreeBytes, true
		}
	}
	return minFree, found
}

func coveringMount(disks []resources.Disk, path string) (resources.Disk, bool) {
	best, found := resources.Disk{}, false
	for _, d := range disks {
		if mountCovers(d.Mount, path) && (!found || len(d.Mount) > len(best.Mount)) {
			best, found = d, true
		}
	}
	return best, found
}

func mountCovers(mount, path string) bool {
	if mount == "" || path == "" {
		return false
	}
	sep := "/"
	if strings.Contains(mount, `\`) || (len(mount) >= 2 && mount[1] == ':') {
		sep, mount, path = `\`, strings.ToLower(mount), strings.ToLower(path)
	}
	mount = strings.TrimSuffix(mount, sep)
	return path == mount || strings.HasPrefix(path, mount+sep)
}
