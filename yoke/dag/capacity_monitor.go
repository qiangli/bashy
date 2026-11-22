package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/resources"
	"github.com/qiangli/yoke/pkg/weave"
	"os"
	"sort"
	"strconv"
	"time"
)

const sprintMonitorSchema = "bashy-sprint-monitor-v1"

type sprintMonitorOptions struct {
	Sprint                int64
	Provider, Model, Host string
	Refresh               bool
}

type sprintMonitorSnapshot struct {
	SchemaVersion string                     `json:"schema_version"`
	Origin        string                     `json:"origin,omitempty"`
	At            time.Time                  `json:"at"`
	Sprint        int64                      `json:"sprint,omitempty"`
	Host          *resources.HostObservation `json:"host"`
	Models        *llmbudget.Report          `json:"models"`
	Inventory     *weave.SprintInventory     `json:"inventory"`
	Alerts        []sprintResourceAlert      `json:"alerts"`
	Warnings      []string                   `json:"warnings"`
}

var observeSprintMonitorRemote func(context.Context, sprintMonitorOptions) (*sprintMonitorSnapshot, error)

func collectSprintMonitor(ctx context.Context, opt sprintMonitorOptions) (*sprintMonitorSnapshot, error) {
	hostname, _ := os.Hostname()
	if opt.Host != "" && opt.Host != "local" && opt.Host != hostname {
		if observeSprintMonitorRemote != nil {
			return observeSprintMonitorRemote(ctx, opt)
		}
		return nil, fmt.Errorf("remote observation for %q is unavailable: no authorized observation transport is configured", opt.Host)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := &sprintMonitorSnapshot{SchemaVersion: sprintMonitorSchema, At: time.Now().UTC(), Sprint: opt.Sprint}
	inv, err := weave.ReadSprintInventory(ctx, resources.ResourcesStateDir())
	if err != nil {
		out.Warnings = append(out.Warnings, "workload attribution: "+err.Error())
	} else {
		out.Inventory = inv
	}
	if opt.Sprint > 0 {
		found := false
		if inv != nil {
			for _, s := range inv.Sprints {
				if s.ID == opt.Sprint {
					found = true
					break
				}
			}
		}
		if inv != nil && inv.Complete && !found {
			return nil, fmt.Errorf("sprint #%d not found", opt.Sprint)
		}
	}
	var workloads []resources.WorkloadRef
	var roots []resources.ScanRoot
	var active []llmbudget.Attribution
	if inv != nil {
		for _, row := range inv.Workloads {
			workloads = append(workloads, resources.WorkloadRef{ID: row.ID, Agent: row.Agent, Sprint: strconv.FormatInt(row.Sprint, 10), Run: strconv.FormatInt(row.Run, 10), Workspace: row.Workspace, Root: resources.ProcessIdentity{PID: row.PID, StartID: row.StartID}, RegisteredAt: row.StartedAt})
			if row.Workspace != "" {
				roots = append(roots, resources.ScanRoot{Path: row.Workspace, WorkloadID: row.ID, Kind: "workspace"})
			}
			active = append(active, llmbudget.Attribution{Agent: row.Agent, Model: row.Model, Run: row.ID, Sprint: row.Sprint, Host: hostname, Active: true})
		}
	}
	// Both sources share the same outer deadline and do not wait on one another.
	type hostResult struct {
		value *resources.HostObservation
		err   error
	}
	type modelResult struct {
		value *llmbudget.Report
		err   error
	}
	hc := make(chan hostResult, 1)
	mc := make(chan modelResult, 1)
	go func() {
		v, e := resources.ObserveHost(ctx, resources.HostObserveOptions{Workloads: workloads, ScanRoots: roots})
		hc <- hostResult{v, e}
	}()
	go func() {
		v, e := llmbudget.CollectReport(ctx, llmbudget.ReportOptions{Provider: opt.Provider, Model: opt.Model, Refresh: opt.Refresh, Active: active})
		mc <- modelResult{v, e}
	}()
	for hc != nil || mc != nil {
		select {
		case r := <-hc:
			out.Host = r.value
			if r.err != nil {
				out.Warnings = append(out.Warnings, "host: "+r.err.Error())
			}
			hc = nil
		case r := <-mc:
			out.Models = r.value
			if r.err != nil {
				out.Warnings = append(out.Warnings, "accounts: "+r.err.Error())
			}
			mc = nil
		case <-ctx.Done():
			out.Warnings = append(out.Warnings, "observation deadline reached; remaining sources unavailable")
			hc = nil
			mc = nil
		}
	}
	ledger, err := resources.ReadAlertState(ctx, resources.ResourcesStateDir())
	if err == nil {
		out.Alerts = activeSprintAlerts(ledger, opt.Sprint, out.At)
	} else if !os.IsNotExist(err) {
		out.Warnings = append(out.Warnings, "alerts: "+err.Error())
	}
	boundSprintMonitor(out)
	return out, nil
}

func boundSprintMonitor(s *sprintMonitorSnapshot) {
	if s.Host != nil {
		h := *s.Host
		s.Host = &h
		if len(h.Processes) > 32 {
			h.Processes = topSprintProcesses(h.Processes, 32)
			s.Warnings = append(s.Warnings, "process details limited to 32; host totals include other processes")
		}
		if s.Sprint > 0 {
			h.Workloads = append([]resources.WorkloadObservation(nil), h.Workloads...)
			selected := strconv.FormatInt(s.Sprint, 10)
			sort.SliceStable(h.Workloads, func(i, j int) bool { return h.Workloads[i].Sprint == selected && h.Workloads[j].Sprint != selected })
		}
		if len(h.Workloads) > 64 {
			h.Workloads = h.Workloads[:64]
			s.Warnings = append(s.Warnings, "workload details limited to 64")
		}
		if len(h.Directories) > 32 {
			h.Directories = h.Directories[:32]
			s.Warnings = append(s.Warnings, "directory details limited to 32")
		}
	}
	if s.Models != nil && len(s.Models.Accounts) > 64 {
		r := *s.Models
		s.Models = &r
		r.Accounts = r.Accounts[:64]
		s.Warnings = append(s.Warnings, "account details limited to 64; roster has additional accounts")
	}
}

func topSprintProcesses(rows []resources.ProcessObservation, limit int) []resources.ProcessObservation {
	if limit <= 0 {
		return nil
	}
	indices := make([]int, 0, limit)
	for i := range rows {
		pos := len(indices)
		for j, k := range indices {
			a, b := rows[i].CPU.Value, rows[k].CPU.Value
			if a != nil && (b == nil || *a > *b) {
				pos = j
				break
			}
		}
		if pos >= limit {
			continue
		}
		if len(indices) < limit {
			indices = append(indices, 0)
		}
		copy(indices[pos+1:], indices[pos:len(indices)-1])
		indices[pos] = i
	}
	out := make([]resources.ProcessObservation, len(indices))
	for i, k := range indices {
		out[i] = rows[k]
	}
	return out
}

type sprintResourceAlert struct {
	ID        string    `json:"id"`
	Sprint    int64     `json:"sprint"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	At        time.Time `json:"at"`
	Owner     string    `json:"owner"`
	Coalesced int       `json:"coalesced,omitempty"`
}

type sprintAlertCondition struct {
	Version    int                   `json:"version"`
	Sprint     int64                 `json:"sprint"`
	Key        string                `json:"key"`
	Owner      string                `json:"owner"`
	Severity   string                `json:"severity"`
	Onset      time.Time             `json:"onset"`
	Recovery   time.Time             `json:"recovery"`
	ObservedAt time.Time             `json:"observed_at"`
	Generation uint64                `json:"generation"`
	Pending    []sprintResourceAlert `json:"pending"`
	Current    *sprintResourceAlert  `json:"current,omitempty"`
}

const sprintAlertObservationTTL = time.Hour

func activeSprintAlerts(ledger *resources.AlertLedger, id int64, now time.Time) []sprintResourceAlert {
	var alerts []sprintResourceAlert
	if ledger == nil {
		return alerts
	}
	for _, raw := range ledger.Entries {
		var c sprintAlertCondition
		if json.Unmarshal(raw, &c) != nil || c.Version != 1 || (id > 0 && c.Sprint != id) {
			continue
		}
		if c.Current != nil && (c.ObservedAt.IsZero() || now.Sub(c.ObservedAt) <= sprintAlertObservationTTL) {
			alerts = append(alerts, *c.Current)
		}
	}
	// Deterministic keys make hash-based change detection independent of map order.
	for i := 1; i < len(alerts); i++ {
		for j := i; j > 0 && alerts[j].ID < alerts[j-1].ID; j-- {
			alerts[j], alerts[j-1] = alerts[j-1], alerts[j]
		}
	}
	return alerts
}
