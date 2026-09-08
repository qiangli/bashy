package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/resources"
	"github.com/qiangli/coreutils/pkg/weave"
	"github.com/spf13/cobra"
)

// Hard launch constraints are explicit operator policy. Advisory monitor
// thresholds never acquire authority to pause, prune, or reject work.
type hostAdmissionPolicy struct {
	Version                 int      `json:"version"`
	MaxCPUPercent           *float64 `json:"max_cpu_percent,omitempty"`
	MinAvailableMemoryBytes *uint64  `json:"min_available_memory_bytes,omitempty"`
	MinFreeDiskBytes        *uint64  `json:"min_free_disk_bytes,omitempty"`
	DiskPath                string   `json:"disk_path,omitempty"`
	AllowEstimatedCPU       bool     `json:"allow_estimated_cpu,omitempty"`
	AllowEstimatedMemory    bool     `json:"allow_estimated_memory,omitempty"`
}

func configureWeaveResourceAdmission(cmd *cobra.Command) {
	policyPath := strings.TrimSpace(os.Getenv("BASHY_HOST_ADMISSION_POLICY"))
	hooks := weave.WeaveResourceHooks{RequireHostCheck: policyPath != "", LookupIdentity: func(ctx context.Context, pid int) (string, error) {
		id, err := resources.LookupProcessIdentity(ctx, pid)
		return id.StartID, err
	}}
	if policyPath != "" {
		hooks.CheckHost = func(ctx context.Context, demand weave.WeaveResourceDemand) error {
			f, err := os.Open(policyPath)
			if err != nil {
				return err
			}
			defer f.Close()
			var p hostAdmissionPolicy
			dec := json.NewDecoder(io.LimitReader(f, 65537))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&p); err != nil {
				return err
			}
			if dec.Decode(new(any)) != io.EOF {
				return errors.New("host admission policy must contain one bounded JSON object")
			}
			if p.Version != 1 {
				return errors.New("unsupported host admission policy version")
			}
			if p.MaxCPUPercent != nil && (*p.MaxCPUPercent <= 0 || *p.MaxCPUPercent > 100) {
				return errors.New("max_cpu_percent must be in (0,100]")
			}
			obs, err := resources.ObserveHost(ctx, resources.HostObserveOptions{})
			if err != nil {
				return err
			}
			return checkHostAdmission(p, obs, demand, time.Now())
		}
	}
	weave.WithWeaveResources(cmd, hooks)
}
func checkHostAdmission(p hostAdmissionPolicy, obs *resources.HostObservation, demand weave.WeaveResourceDemand, now time.Time) error {
	if obs == nil || obs.System == nil {
		return errors.New("host observation unavailable")
	}
	require := func(section string, allowEstimated bool) error {
		s, ok := obs.Sections[section]
		if !ok || s.Stale || s.Kind == "unknown" || !now.Before(s.ExpiresAt) || s.At.After(now.Add(time.Second)) {
			return fmt.Errorf("%s observation unavailable or stale", section)
		}
		if s.Kind != "actual" && !allowEstimated {
			return fmt.Errorf("%s observation is estimated; policy requires actual", section)
		}
		return nil
	}
	if p.MaxCPUPercent != nil {
		if err := require("cpu", p.AllowEstimatedCPU); err != nil {
			return err
		}
		if obs.System.CPU.Source == "ticks-boot" {
			return errors.New("CPU rate has no recent sample window")
		}
		if obs.System.CPU.UsagePercent >= *p.MaxCPUPercent {
			return errors.New("host CPU pressure exceeds configured ceiling")
		}
	}
	if p.MinAvailableMemoryBytes != nil {
		if demand.MemoryBytes == 0 {
			return errors.New("run memory demand unknown under configured hard memory floor")
		}
		if err := require("memory", p.AllowEstimatedMemory); err != nil {
			return err
		}
		available := obs.System.Memory.AvailableBytes
		if available < *p.MinAvailableMemoryBytes || available-*p.MinAvailableMemoryBytes < demand.MemoryBytes {
			return errors.New("host memory headroom below configured floor plus run demand")
		}
	}
	if p.MinFreeDiskBytes != nil {
		if err := require("disks", false); err != nil {
			return err
		}
		path := p.DiskPath
		if path == "" {
			path = demand.Queue
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		selected := -1
		best := -1
		for i, disk := range obs.System.Disks {
			rel, err := filepath.Rel(disk.Mount, path)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(disk.Mount) > best {
				selected = i
				best = len(disk.Mount)
			}
		}
		if selected < 0 {
			return errors.New("destination filesystem observation unavailable")
		}
		if obs.System.Disks[selected].FreeBytes < *p.MinFreeDiskBytes {
			return errors.New("destination disk free bytes below configured floor")
		}
	}
	return nil
}
