// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/dag"
	"github.com/qiangli/yoke/pkg/resources"
	"github.com/qiangli/yoke/pkg/shape/slot"
)

var (
	capacityObserveHost = resources.ObserveHost
	capacityLoadPolicy  = dag.LoadCapacityPolicy
)

// dispatchDag runs `bashy dag` in-process so a fresh install or isolated HOME
// never requires a Go toolchain, module cache, or source checkout before
// executing a lifecycle dag.md, while still honoring active `shape` slots and
// explicit `BASHY_ADAPTERS_ROOT` overrides.
func dispatchDag(invokedAs string, args []string) int {
	if invokedAs == "" {
		invokedAs = "dag"
	}
	if os.Getenv(adapterInprocEnv) == "1" {
		return runDagInproc(args)
	}
	if !certProfile() {
		for _, slotName := range uniqueStrings(invokedAs, "dag") {
			if active, ok, err := shapeSlotSelect(slotName); err == nil && ok && active.Path != "" {
				env, envErr := slot.EntryEnvironment(active.Path, appendAdapterEnv(os.Environ(), invokedAs, false))
				if envErr == nil {
					return shapeSlotLaunch(active.Path, append([]string{invokedAs}, args...), env)
				}
			}
		}
	}
	if strings.TrimSpace(os.Getenv(adaptersRootEnv)) != "" {
		return runCommandAdapter("yoke", "dag", invokedAs, args)
	}
	return runDagInproc(args)
}

func runDagInproc(args []string) int {
	self := strings.TrimSpace(os.Getenv("BASHY_SELF"))
	if self == "" {
		self = strings.TrimSpace(os.Getenv(adapterSelfEnv))
	}
	for _, k := range []string{adapterInprocEnv, adapterExecEnv, adapterSelfEnv, invokedAsEnv, "BASHY_SELF"} {
		_ = os.Unsetenv(k)
	}
	if self != "" && len(os.Args) > 0 {
		os.Args[0] = self
	}
	applyAllProxyFallback()
	return runDagDispatch(args, os.Stdout, os.Stderr)
}

// runDagDispatch mounts the DAG runner with the capacity endpoint and runs it.
//
// Two things here are load-bearing. dag.NewDagCmd sets SilenceErrors, so an
// error cobra raises BEFORE RunE (an unknown flag, a parse failure — and, until
// coreutils 539b8fa7 declared the root's positional contract, every target
// once `capacity` was mounted) reaches this caller as the only evidence that
// anything went wrong. Mapping it straight to an exit code, as this used to,
// produced a silent `exit 1` that two sprint-163 workers spent their last ten
// minutes on. dag's own failures are *dag.Error values that emitErr has
// already written as an envelope; those are not printed twice.
func runDagDispatch(args []string, stdout, stderr io.Writer) int {
	cmd := dag.NewDagCmd()
	dag.AddCapacityCommands(cmd, sprintCapacityServices())
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	setCobraArgs(cmd, args)
	err := cmd.Execute()
	var dagErr *dag.Error
	if err != nil && !errors.As(err, &dagErr) {
		fmt.Fprintf(stderr, "bashy dag: %v\n", err)
	}
	return dag.ExitCodeOf(err)
}

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
			snapshot, e := collectSprintMonitor(ctx, sprintMonitorOptions{})
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
