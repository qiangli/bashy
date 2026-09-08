package agentos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/qiangli/coreutils/pkg/llmbudget"
	"github.com/qiangli/coreutils/pkg/resources"
	"github.com/qiangli/coreutils/pkg/weave"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const sprintMonitorSchema = "bashy-sprint-monitor-v1"

type sprintMonitorOptions struct {
	Sprint                int64
	Provider, Model, Host string
	Refresh               bool
}
type sprintMonitorSnapshot struct {
	SchemaVersion string                     `json:"schema_version"`
	At            time.Time                  `json:"at"`
	Sprint        int64                      `json:"sprint,omitempty"`
	Host          *resources.HostObservation `json:"host"`
	Models        *llmbudget.Report          `json:"models"`
	Inventory     *weave.SprintInventory     `json:"inventory"`
	Alerts        []sprintResourceAlert      `json:"alerts"`
	Warnings      []string                   `json:"warnings"`
}
type sprintMonitorRuntime struct {
	collect func(context.Context, sprintMonitorOptions) (*sprintMonitorSnapshot, error)
	now     func() time.Time
	wait    func(context.Context, time.Duration) error
}

func defaultSprintMonitorRuntime() sprintMonitorRuntime {
	return sprintMonitorRuntime{collect: collectSprintMonitor, now: time.Now, wait: waitInboxPoll}
}

func newSprintMonitorCmd() *cobra.Command { return sprintMonitorCommand(defaultSprintMonitorRuntime()) }
func sprintMonitorCommand(rt sprintMonitorRuntime) *cobra.Command {
	var opt sprintMonitorOptions
	var watch, jsonOut bool
	var interval, duration time.Duration
	cmd := &cobra.Command{Use: "monitor [sprint-id]", Short: "Observe shared host and account resources, including competing work", Args: cobra.MaximumNArgs(1)}
	cmd.Flags().BoolVar(&watch, "watch", false, "follow compact resource changes; never launch or pause work")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit a versioned snapshot or bounded NDJSON changes")
	cmd.Flags().DurationVar(&interval, "interval", 5*time.Second, "watch cadence (minimum 5s; shared source caches still apply)")
	cmd.Flags().DurationVar(&duration, "duration", 0, "stop watching after this duration (zero means until cancelled)")
	bindMonitorFilters(cmd, &opt)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			id, err := parseSprintID(args[0])
			if err != nil {
				return err
			}
			opt.Sprint = id
		}
		if interval < 5*time.Second {
			return fmt.Errorf("monitor interval must be at least 5s")
		}
		if duration < 0 {
			return fmt.Errorf("monitor duration cannot be negative")
		}
		if !watch && cmd.Flags().Changed("duration") {
			return fmt.Errorf("--duration requires --watch")
		}
		ctx, stop := sprintMonitorContext(cmd.Context(), duration)
		defer stop()
		return runSprintMonitor(ctx, cmd.OutOrStdout(), opt, watch, jsonOut, monitorTTY(cmd.OutOrStdout()), interval, rt)
	}
	return cmd
}
func bindMonitorFilters(cmd *cobra.Command, opt *sprintMonitorOptions) {
	cmd.Flags().StringVar(&opt.Provider, "provider", "", "show provider accounts while retaining shared-pool competitors")
	cmd.Flags().StringVar(&opt.Provider, "vendor", "", "alias for --provider")
	cmd.Flags().StringVar(&opt.Model, "model", "", "select model detail without splitting shared account capacity")
	cmd.Flags().StringVar(&opt.Host, "host", "", "registered host target (default local)")
	cmd.Flags().BoolVar(&opt.Refresh, "refresh", false, "request due source refresh, respecting throttling")
}
func monitorTTY(w io.Writer) bool { f, ok := w.(*os.File); return ok && term.IsTerminal(int(f.Fd())) }
func sprintMonitorContext(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	if d > 0 {
		limited, cancel := context.WithTimeout(ctx, d)
		return limited, func() { cancel(); stop() }
	}
	return ctx, stop
}

// Remote observation is a distinct authorized transport seam. A registered name
// alone grants no data transfer or job dispatch, so absence is explicit.
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
		out.Alerts = activeSprintAlerts(ledger, opt.Sprint)
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
			h.Processes = append([]resources.ProcessObservation(nil), h.Processes...)
			sort.SliceStable(h.Processes, func(i, j int) bool {
				a, b := h.Processes[i].CPU.Value, h.Processes[j].CPU.Value
				return a != nil && (b == nil || *a > *b)
			})
			h.Processes = h.Processes[:32]
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
func summarizeSprintMonitor(s *sprintMonitorSnapshot) weave.SprintResourceSummary {
	r := weave.SprintResourceSummary{At: s.At, Status: "available", Read: "bashy sprint monitor", Alerts: len(s.Alerts), Warnings: s.Warnings}
	if s.Sprint > 0 {
		r.Read += " " + strconv.FormatInt(s.Sprint, 10)
	}
	if s.Host == nil {
		r.Unknown++
		r.Status = "partial"
	} else {
		if s.Host.System != nil {
			r.Host = s.Host.System.Host
		}
		for _, st := range s.Host.Sections {
			if st.Kind == "unknown" || st.Stale {
				r.Unknown++
			}
		}
	}
	if s.Models == nil {
		r.Unknown++
		r.Status = "partial"
	} else {
		r.Accounts = len(s.Models.Accounts)
		for _, a := range s.Models.Accounts {
			if a.Status != "ok" {
				r.Unknown++
			}
		}
	}
	if s.Host != nil && s.Host.System != nil {
		st := s.Host.Sections["cpu"]
		r.CPUKind = st.Kind
		if st.Kind != "unknown" && !st.Stale && s.Host.System.CPU.Source != "" {
			v := math.Round(s.Host.System.CPU.UsagePercent*10) / 10
			r.CPU = &v
		}
		st = s.Host.Sections["memory"]
		if st.Kind != "unknown" && !st.Stale && s.Host.System.Memory.TotalBytes > 0 {
			v := math.Round(s.Host.System.Memory.UsedPercent*10) / 10
			r.Memory = &v
		}
	}
	if r.Unknown > 0 || len(r.Warnings) > 0 {
		r.Status = "partial"
	}
	return r
}
func sprintMonitorSummary(ctx context.Context, id int64) (weave.SprintResourceSummary, error) {
	s, e := collectSprintMonitor(ctx, sprintMonitorOptions{Sprint: id})
	if e != nil {
		return weave.SprintResourceSummary{}, e
	}
	return summarizeSprintMonitor(s), nil
}

type sprintMonitorAccount struct {
	Provider string             `json:"provider"`
	Account  string             `json:"account"`
	Pool     string             `json:"pool"`
	Status   string             `json:"status"`
	Metrics  []llmbudget.Metric `json:"metrics,omitempty"`
}

func monitorAccountLevels(s *sprintMonitorSnapshot) []sprintMonitorAccount {
	var accounts []sprintMonitorAccount
	if s.Models == nil {
		return accounts
	}
	for _, a := range s.Models.Accounts {
		row := sprintMonitorAccount{Provider: a.Provider, Account: a.Account, Pool: a.Pool, Status: a.Status}
		for _, m := range a.Metrics {
			if strings.HasPrefix(m.Name, "quota.") || strings.HasPrefix(m.Name, "budget.") || strings.HasPrefix(m.Name, "billing.") {
				m.ObservedAt = time.Time{} // shared report retains exact source timestamps
				row.Metrics = append(row.Metrics, m)
			}
		}
		accounts = append(accounts, row)
	}
	return accounts
}

type sprintMonitorEvent struct {
	SchemaVersion string                      `json:"schema_version"`
	Type          string                      `json:"type"`
	At            time.Time                   `json:"at"`
	Summary       weave.SprintResourceSummary `json:"summary"`
	Alerts        []sprintResourceAlert       `json:"alerts,omitempty"`
	Pending       int                         `json:"pending,omitempty"`
	Accounts      []sprintMonitorAccount      `json:"accounts,omitempty"`
}

func runSprintMonitor(ctx context.Context, w io.Writer, opt sprintMonitorOptions, watch, jsonOut, tty bool, interval time.Duration, rt sprintMonitorRuntime) error {
	var previous string
	var lastOutput time.Time
	for {
		if ctx.Err() != nil {
			return nil
		}
		s, err := rt.collect(ctx, opt)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if !watch {
			if jsonOut {
				return json.NewEncoder(w).Encode(s)
			}
			return renderSprintMonitor(w, s)
		}
		if s.Inventory != nil {
			targets := map[int64]string{}
			for _, seat := range s.Inventory.Sprints {
				if seat.Active && seat.Owner != "" && (opt.Sprint == 0 || seat.ID == opt.Sprint) {
					targets[seat.ID] = seat.Owner
				}
			}
			if len(targets) > 0 {
				alerts, alertErr := updateSprintAlertTargets(ctx, s, targets, opt.Sprint, true)
				if alertErr != nil {
					s.Warnings = append(s.Warnings, "resource notice pending: "+alertErr.Error())
				}
				s.Alerts = alerts
			}
		}
		e := sprintMonitorEvent{SchemaVersion: sprintMonitorSchema, Type: "change", At: rt.now().UTC(), Summary: summarizeSprintMonitor(s), Alerts: s.Alerts, Accounts: monitorAccountLevels(s)}
		// Fresh timestamps do not make a material change. Resource levels are
		// summarized in the status text; alert transitions retain their IDs.
		stable := e
		stable.At = time.Time{}
		stable.Summary.At = time.Time{}
		b, _ := json.Marshal(stable)
		hash := sha256.Sum256(b)
		fingerprint := hex.EncodeToString(hash[:])
		changed := fingerprint != previous
		if changed || lastOutput.IsZero() || rt.now().Sub(lastOutput) >= time.Minute {
			if previous == "" {
				e.Type = "snapshot"
			} else if !changed {
				e.Type = "heartbeat"
			}
			data, err := boundedMonitorEvent(e)
			if err != nil {
				return err
			}
			if jsonOut {
				_, err = w.Write(append(data, '\n'))
			} else if tty {
				_, err = fmt.Fprint(w, "\x1b[H\x1b[2J")
				if err == nil {
					err = renderSprintMonitor(w, s)
				}
			} else {
				_, err = fmt.Fprintf(w, "%s %s: %s; %d accounts, %d alerts, %d unknown; %s\n", e.At.Format(time.RFC3339), e.Type, e.Summary.Status, e.Summary.Accounts, e.Summary.Alerts, e.Summary.Unknown, e.Summary.Read)
			}
			if err != nil {
				return err
			}
			lastOutput = rt.now()
			previous = fingerprint
		}
		if err := rt.wait(ctx, interval); err != nil {
			return nil
		}
	}
}
func boundedMonitorEvent(e sprintMonitorEvent) ([]byte, error) {
	for {
		b, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		if len(b) <= 4095 {
			return b, nil
		}
		if len(e.Accounts) > 0 {
			e.Accounts = e.Accounts[:len(e.Accounts)-1]
			e.Pending++
			continue
		}
		if len(e.Alerts) > 0 {
			e.Alerts = e.Alerts[:len(e.Alerts)-1]
			e.Pending++
			continue
		}
		if len(e.Summary.Warnings) > 0 {
			e.Summary.Warnings = e.Summary.Warnings[:len(e.Summary.Warnings)-1]
			e.Pending++
			continue
		}
		return nil, fmt.Errorf("monitor summary exceeds output budget")
	}
}
func renderSprintMonitor(out io.Writer, s *sprintMonitorSnapshot) error {
	var text strings.Builder
	var w io.Writer = &text
	summary := summarizeSprintMonitor(s)
	if _, err := fmt.Fprintf(w, "resources %s — host %s; %d accounts; %d alerts; %d unknown\n", summary.Status, summary.Host, summary.Accounts, summary.Alerts, summary.Unknown); err != nil {
		return err
	}
	if s.Sprint > 0 {
		fmt.Fprintf(w, "selected sprint #%d; shared host and account competitors remain included\n", s.Sprint)
	}
	if s.Host != nil && s.Host.System != nil {
		cpu, memory := "unknown", "unknown"
		if summary.CPU != nil {
			cpu = fmt.Sprintf("%.1f%% (%s)", *summary.CPU, s.Host.System.CPU.Source)
		}
		if summary.Memory != nil {
			memory = fmt.Sprintf("%.1f%%", *summary.Memory)
		}
		fmt.Fprintf(w, "CPU %s; memory %s; %d processes sampled\n", cpu, memory, s.Host.ProcessCoverage.Included)
		for i, row := range s.Host.Workloads {
			if i >= 10 {
				fmt.Fprintf(w, "more workload detail: sprint monitor --json\n")
				break
			}
			cpu = "unknown"
			if row.CPU.Value != nil {
				cpu = fmt.Sprintf("%.1f%%", *row.CPU.Value)
			}
			fmt.Fprintf(w, "run %s sprint=%s agent=%s CPU %s (%s)\n", row.Run, row.Sprint, row.Agent, cpu, row.CPU.Status.Kind)
		}
	}
	if s.Models != nil {
		for _, a := range s.Models.Accounts {
			fmt.Fprintf(w, "%s account=%s pool=%s %s [%s]; %s\n", a.Provider, a.Account, a.Pool, a.Lane, a.Status, strings.Join(a.Models, ", "))
			for _, m := range a.Metrics {
				if strings.HasPrefix(m.Name, "quota.") || strings.HasPrefix(m.Name, "budget.") || strings.HasPrefix(m.Name, "billing.") {
					value := "unknown"
					if m.Value != nil {
						value = fmt.Sprintf("%.3g %s", *m.Value, m.Unit)
					}
					fmt.Fprintf(w, "  %s: %s [%s; %s]", m.Name, value, m.Classification, m.Source)
					if m.ResetAt != nil {
						fmt.Fprintf(w, " resets %s", m.ResetAt.Format(time.RFC3339))
					}
					fmt.Fprintln(w)
				}
			}
		}
	}
	for _, a := range s.Alerts {
		fmt.Fprintf(w, "%s: %s (%s)\n", a.Severity, a.Message, a.ID)
	}
	for _, warning := range s.Warnings {
		fmt.Fprintf(w, "unavailable: %s\n", warning)
	}
	_, err := io.WriteString(out, text.String())
	return err
}
