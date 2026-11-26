package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/agentpty"
	"github.com/qiangli/yoke/pkg/chat"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/spf13/cobra"
)

const workerStatusSchema = "bashy-worker-status-v1"

type workerStatusSnapshot struct {
	SchemaVersion string                    `json:"schema_version"`
	At            time.Time                 `json:"at"`
	Workers       []workerStatus            `json:"workers"`
	Accounts      []llmbudget.AccountReport `json:"accounts"`
	Warnings      []string                  `json:"warnings"`
}

type workerStatus struct {
	ID          string     `json:"id"`
	Binding     string     `json:"binding"`
	Task        string     `json:"task,omitempty"`
	State       string     `json:"state"`
	StateSource string     `json:"state_source"`
	Limit       string     `json:"limit"`
	LimitSource string     `json:"limit_source"`
	ResetAt     *time.Time `json:"reset_at,omitempty"`
}

func newWorkerStatusCmd() *cobra.Command {
	return workerStatusCommand(workerStatusRuntime{collect: collectWorkerStatus, wait: waitInboxPoll})
}

func workerStatusCommand(rt workerStatusRuntime) *cobra.Command {
	var asJSON, diff bool
	var duration time.Duration
	cmd := &cobra.Command{
		Use:   "worker-status",
		Short: "Show or watch local worker sessions and available account readings",
		Long:  "Observe local worker sessions and cached account readings. Account readings describe pools independently of the session rows.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("watch") {
				if duration <= 0 {
					return sprintUsagef("--watch requires a positive duration")
				}
				ctx, stop := sprintMonitorContext(cmd.Context(), duration)
				defer stop()
				return watchWorkerStatus(ctx, cmd.OutOrStdout(), rt, asJSON, diff)
			}
			if diff {
				return sprintUsagef("--diff requires --watch")
			}
			snapshot, err := rt.collect(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(snapshot)
			}
			return renderWorkerStatus(cmd.OutOrStdout(), snapshot)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit a versioned JSON snapshot or newline-delimited watch events")
	cmd.Flags().DurationVar(&duration, "watch", 0, "watch for a positive duration, sampling every 5s")
	cmd.Flags().BoolVar(&diff, "diff", false, "show only changes after the first snapshot (requires --watch)")
	return cmd
}

func collectWorkerStatus(ctx context.Context) (workerStatusSnapshot, error) {
	now := time.Now().UTC()
	snapshot := workerStatusSnapshot{
		SchemaVersion: workerStatusSchema, At: now,
		Workers: []workerStatus{}, Accounts: []llmbudget.AccountReport{}, Warnings: []string{},
	}
	members, err := room.PeekMembers()
	if err != nil {
		snapshot.Warnings = append(snapshot.Warnings, "worker sessions unavailable: room membership could not be read")
	}
	catalog := fleet.New()
	for _, member := range members {
		if err := ctx.Err(); err != nil {
			return snapshot, err
		}
		snapshot.Workers = append(snapshot.Workers, readWorkerStatus(member, catalog, now))
	}
	sort.Slice(snapshot.Workers, func(i, j int) bool {
		return snapshot.Workers[i].ID < snapshot.Workers[j].ID
	})
	report, err := llmbudget.CollectReport(llmbudget.WithCachedSourcesOnly(ctx), llmbudget.ReportOptions{Now: now})
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	if err != nil {
		snapshot.Warnings = append(snapshot.Warnings, "account readings unavailable: the local usage report could not be read")
	} else {
		snapshot.Accounts = report.Accounts
		snapshot.Warnings = append(snapshot.Warnings, report.Warnings...)
	}
	return snapshot, nil
}

func readWorkerStatus(member room.Card, catalog *fleet.Catalog, now time.Time) workerStatus {
	worker := workerStatus{
		ID: member.ID, Binding: member.Binding, Task: room.Brief(member.Task, 160),
		State: "unavailable", StateSource: "no current session signal",
		Limit: "unavailable", LimitSource: "no current session limit signal",
	}
	joined, err := time.Parse(time.RFC3339, member.Joined)
	if err != nil {
		return worker
	}
	turn, turnAt := readWorkerTurn(member.EventsPath, joined, now)
	if turn != "" {
		worker.State, worker.StateSource = turn, "structured session events"
	}

	tail, logAt := readWorkerFileTail(member.LogPath, 16<<10)
	if logAt.Before(joined) || logAt.Before(turnAt) || logAt.After(now.Add(time.Second)) || now.Sub(logAt) > 2*time.Minute {
		return worker
	}
	if gate := agentpty.VisibleDialogGate(string(tail)); gate.Kind != agentpty.GateNone {
		worker.State, worker.StateSource = "dialog", "recent terminal output: "+string(gate.Kind)
	}

	tool, _ := catalog.Tool(member.Tool)
	model, _ := catalog.Model(member.Model)
	plan, _ := catalog.Plan(model.Plan)
	lastLine := bytes.TrimSpace(tail)
	if i := bytes.LastIndexByte(lastLine, '\n'); i >= 0 {
		lastLine = lastLine[i+1:]
	}
	signals := append(append([]fleet.UsageSignal(nil), tool.UsageSignals...), plan.UsageSignals...)
	for _, signal := range fleet.MatchSignals(signals, agentpty.ScreenText(string(lastLine)), nil, logAt.Local()) {
		switch signal.Kind {
		case fleet.UsageSignalKindExhausted, fleet.UsageSignalKindRateLimited:
		default:
			continue
		}
		if signal.ResetAt != nil && !signal.ResetAt.After(now) {
			continue
		}
		worker.Limit, worker.LimitSource, worker.ResetAt = signal.Kind, "latest terminal line: "+signal.Signal, signal.ResetAt
		if worker.State == "unavailable" {
			worker.State, worker.StateSource = "limit-observed", worker.LimitSource
		}
		break
	}
	return worker
}

func readWorkerTurn(path string, joined, now time.Time) (string, time.Time) {
	tail, modified := readWorkerFileTail(path, 64<<10)
	if modified.Before(joined) {
		return "", time.Time{}
	}
	// The first line in a truncated tail can be partial and the last line
	// can be in flight. Decode complete records only.
	var state string
	var at time.Time
	for len(tail) > 0 {
		line, rest, ok := bytes.Cut(tail, []byte{'\n'})
		if !ok {
			break
		}
		tail = rest
		var event chat.Event
		if err := json.Unmarshal(line, &event); err != nil || event.Timestamp.Before(joined) || event.Timestamp.Before(at) || event.Timestamp.After(now.Add(time.Second)) {
			continue
		}
		switch event.Type {
		case chat.EventTurnStart:
			state, at = "working", event.Timestamp
		case chat.EventTurnEnd:
			var end struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(event.Data, &end); err != nil || end.Status == "" {
				continue
			}
			state, at = "turn-ended", event.Timestamp
		}
	}
	return state, at
}

func readWorkerFileTail(path string, limit int64) ([]byte, time.Time) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, time.Time{}
	}
	file, err := openWorkerStatusFile(path)
	if err != nil {
		return nil, time.Time{}
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, time.Time{}
	}
	size := min(opened.Size(), limit)
	tail := make([]byte, size)
	n, err := file.ReadAt(tail, opened.Size()-size)
	if err != nil && err != io.EOF {
		return nil, time.Time{}
	}
	return tail[:n], opened.ModTime()
}

func renderWorkerStatus(out io.Writer, snapshot workerStatusSnapshot) error {
	var text strings.Builder
	fmt.Fprintln(&text, "Workers on this host")
	if len(snapshot.Workers) == 0 {
		fmt.Fprintln(&text, "  No live worker sessions found")
	}
	for _, worker := range snapshot.Workers {
		fmt.Fprintf(&text, "  %q %q — %s (%q), limit %s (%q)", worker.ID, worker.Binding, worker.State, worker.StateSource, worker.Limit, worker.LimitSource)
		if worker.ResetAt != nil {
			fmt.Fprintf(&text, ", resets %s", worker.ResetAt.UTC().Format(time.RFC3339))
		} else if worker.Limit != "unavailable" {
			fmt.Fprint(&text, ", reset unavailable")
		}
		if worker.Task != "" {
			fmt.Fprintf(&text, ", task %q", worker.Task)
		}
		fmt.Fprintln(&text)
	}

	fmt.Fprintln(&text, "Account pools")
	if len(snapshot.Accounts) == 0 {
		fmt.Fprintln(&text, "  Account readings unavailable")
	}
	for _, account := range snapshot.Accounts {
		fmt.Fprintf(&text, "  %q account=%q pool=%q %q [%q]\n", account.Provider, account.Account, account.Pool, account.Lane, account.Status)
		if !account.AccountKnown {
			fmt.Fprintln(&text, "    Account identity unavailable")
		}
		quota, usage, reset := false, false, false
		for _, metric := range account.Metrics {
			isQuota := strings.HasPrefix(metric.Name, "quota.")
			isUsage := strings.HasPrefix(metric.Name, "usage.") || strings.HasPrefix(metric.Name, "billing.")
			if (!isQuota && !isUsage) || (metric.Value == nil && metric.ResetAt == nil) {
				continue
			}
			fmt.Fprintf(&text, "    %q", metric.Name)
			if metric.Window != "" {
				fmt.Fprintf(&text, " (%q)", metric.Window)
			}
			if metric.Value != nil {
				fmt.Fprintf(&text, " = %.6g %q", *metric.Value, metric.Unit)
				quota, usage = quota || isQuota, usage || isUsage
			} else {
				fmt.Fprint(&text, " = unavailable")
			}
			fmt.Fprintf(&text, " [%q; %q]", metric.Classification, metric.Source)
			if metric.ResetAt != nil {
				fmt.Fprintf(&text, ", resets %s", metric.ResetAt.UTC().Format(time.RFC3339))
				reset = true
			}
			fmt.Fprintln(&text)
		}
		if !quota {
			fmt.Fprintln(&text, "    Quota unavailable")
		}
		if !usage {
			fmt.Fprintln(&text, "    Usage or spend unavailable")
		}
		if !reset {
			fmt.Fprintln(&text, "    Reset unavailable")
		}
		for _, limitation := range account.Limitations {
			fmt.Fprintf(&text, "    %q\n", limitation)
		}
	}
	for _, warning := range snapshot.Warnings {
		fmt.Fprintf(&text, "  %q\n", warning)
	}
	_, err := io.WriteString(out, text.String())
	return err
}
