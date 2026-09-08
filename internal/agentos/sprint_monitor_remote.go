package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/dag"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/llmbudget"
	"github.com/qiangli/coreutils/pkg/resources"
)

func init() { observeSprintMonitorRemote = collectRemoteSprintMonitor }
func sprintCapacityServices() dag.CapacityServices {
	return dag.CapacityServices{
		Identity: func(ctx context.Context, pid int) (string, error) {
			id, e := resources.LookupProcessIdentity(ctx, pid)
			return id.StartID, e
		},
		Probe: func(ctx context.Context, worker string) (dag.CapacityObservation, error) {
			h, err := resources.ObserveHost(ctx, resources.HostObserveOptions{})
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
			if st := h.Sections["disks"]; st.Kind == "actual" && !st.Stale && len(sys.Disks) > 0 {
				free := uint64(math.MaxUint64)
				for _, d := range sys.Disks {
					if d.FreeBytes < free {
						free = d.FreeBytes
					}
				}
				facts.Capacities["disk_bytes"] = free
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
func collectRemoteSprintMonitor(ctx context.Context, opt sprintMonitorOptions) (*sprintMonitorSnapshot, error) {
	client, e := dag.NewCapacityClient()
	if e != nil {
		return nil, e
	}
	raw, e := client.Observe(ctx, opt.Host)
	if e != nil {
		return nil, e
	}
	return decodeRemoteSprintMonitor(raw, opt, time.Now())
}
func decodeRemoteSprintMonitor(raw []byte, opt sprintMonitorOptions, now time.Time) (*sprintMonitorSnapshot, error) {
	var snapshot sprintMonitorSnapshot
	if e := json.Unmarshal(raw, &snapshot); e != nil {
		return nil, e
	}
	if snapshot.SchemaVersion != sprintMonitorSchema || snapshot.At.After(now) || now.Sub(snapshot.At) > 10*time.Second {
		return nil, errors.New("remote monitor schema or freshness mismatch")
	}
	snapshot.Sprint = opt.Sprint
	if opt.Sprint > 0 {
		found := false
		if snapshot.Inventory != nil {
			for _, s := range snapshot.Inventory.Sprints {
				if s.ID == opt.Sprint {
					found = true
					break
				}
			}
			if snapshot.Inventory.Complete && !found {
				return nil, errors.New("selected sprint absent on target")
			}
		}
		var alerts []sprintResourceAlert
		for _, a := range snapshot.Alerts {
			if a.Sprint == opt.Sprint {
				alerts = append(alerts, a)
			}
		}
		snapshot.Alerts = alerts
	}
	if snapshot.Models != nil && (opt.Provider != "" || opt.Model != "") {
		wanted := opt.Model
		if model, ok := fleet.New().Model(wanted); ok {
			wanted = model.Name
		}
		var accounts []llmbudget.AccountReport
		for _, a := range snapshot.Models.Accounts {
			if opt.Provider != "" && !strings.EqualFold(a.Provider, opt.Provider) {
				continue
			}
			match := opt.Model == ""
			for _, m := range a.Models {
				if strings.EqualFold(m, wanted) {
					match = true
				}
			}
			if match {
				accounts = append(accounts, a)
			}
		}
		snapshot.Models.Accounts = accounts
	}
	boundSprintMonitor(&snapshot)
	// Remote observation never evaluates or publishes against local sprint owners.
	snapshot.Origin = opt.Host
	return &snapshot, nil
}
