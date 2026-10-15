package agentos

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

func modelQuotaCommand(collect func(context.Context, llmbudget.QuotaOptions) ([]llmbudget.QuotaRow, error), overlay func([]llmbudget.QuotaRow, time.Time) error, clock func() time.Time) *cobra.Command {
	var opt llmbudget.QuotaOptions
	var jsonOut bool
	cmd := &cobra.Command{Use: "quota", Short: "View provider quota, resets and recorded cooldowns", SilenceUsage: true, Args: cobra.NoArgs,
		Long: `View provider quota and recorded seat cooldowns. Only enabled consent-gated probes run.

Fields:
  PROVIDER: configured model provider; PLAN: subscription plan, when known.
  WINDOW: observation window; USED: percentage used; RESETS: local reset time and time remaining.
  TOKENS/BAL: observed tokens or account balance (with its unit).
  SOURCE: observation source; AGE: rounded time since observation; STATUS: availability.
  Unknown cells are '-'. JSON also includes headroom, stale, suggested_workers and classification.`}
	cmd.Flags().BoolVar(&opt.All, "all", false, "include every configured provider")
	cmd.Flags().BoolVar(&opt.Refresh, "refresh", false, "refresh enabled sources (20 second deadline)")
	cmd.Flags().StringVar(&opt.Provider, "provider", "", "filter by provider")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit bashy-registry-list-v1 quota rows")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		timeout := 5 * time.Second
		if opt.Refresh {
			timeout = 20 * time.Second
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		opt.Now = clock()
		rows, err := collect(ctx, opt)
		if err != nil {
			return err
		}
		rows = append([]llmbudget.QuotaRow(nil), rows...)
		if err = overlay(rows, opt.Now); err != nil {
			return err
		}
		if jsonOut {
			items := make([]quotaJSONRow, 0, len(rows))
			for _, r := range rows {
				items = append(items, quotaJSONRow{r, r.Headroom, r.Stale, r.SuggestedWorkers, r.Classification})
			}
			return fleet.WriteListJSON(cmd.OutOrStdout(), "quota", fleet.BasicView(false, opt.All), items)
		}
		return renderModelQuota(cmd.OutOrStdout(), rows, opt.Now, time.Local)
	}
	cmd.AddCommand(modelQuotaTestCommand(resolveQuotaProbes, runQuotaProbe, time.Now), modelQuotaConsentCommand("enable", resolveQuotaProbes, llmbudget.EnableProbe), modelQuotaConsentCommand("disable", resolveQuotaProbes, llmbudget.DisableProbe), modelQuotaCaptureCommand(time.Now))
	return cmd
}

// Keep unknown metadata explicit in the CLI envelope, including false stale.
type quotaJSONRow struct {
	llmbudget.QuotaRow
	Headroom         *float64 `json:"headroom"`
	Stale            bool     `json:"stale"`
	SuggestedWorkers *int     `json:"suggested_workers"`
	Classification   string   `json:"classification"`
}

func quotaCell(s string) string {
	if s == "" {
		return "-"
	}
	return strings.Join(strings.Fields(s), " ")
}
func quotaDuration(d time.Duration) string {
	s := d.Round(time.Minute).String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	return s
}
func renderModelQuota(out io.Writer, rows []llmbudget.QuotaRow, now time.Time, loc *time.Location) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tPLAN\tWINDOW\tUSED\tRESETS\tTOKENS/BAL\tSOURCE\tAGE\tSTATUS")
	for _, r := range rows {
		used, reset, quantity, age := "-", "-", "-", "-"
		if r.UsedPercent != nil {
			used = fmt.Sprintf("%.0f%%", *r.UsedPercent)
		}
		if r.ResetAt != nil {
			relative := "in " + quotaDuration(r.ResetAt.Sub(now))
			if r.ResetAt.Before(now) {
				relative = quotaDuration(now.Sub(*r.ResetAt)) + " ago"
			}
			reset = r.ResetAt.In(loc).Format("Mon 15:04") + " (" + relative + ")"
		}
		if r.Tokens != nil {
			quantity = fmt.Sprintf("%g", *r.Tokens)
		}
		if r.Balance != nil {
			balance := strings.TrimSpace(fmt.Sprintf("%g %s", *r.Balance, r.BalanceUnit))
			if quantity != "-" {
				quantity += " / " + balance
			} else {
				quantity = balance
			}
		}
		if r.ObservedAt != nil {
			age = now.Sub(*r.ObservedAt).Round(time.Second).String()
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", quotaCell(r.Provider), quotaCell(r.Plan), quotaCell(r.Window), used, reset, quotaCell(quantity), quotaCell(r.Source), age, quotaCell(r.Status))
	}
	return w.Flush()
}

func quotaQueueDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var dirs []string
	for _, base := range []string{weave.StateRoot(), filepath.Join(home, ".agents", "weave"), filepath.Join(home, ".agents", "ycode", "weave")} {
		entries, _ := os.ReadDir(base)
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(base, e.Name()))
			}
		}
	}
	return dirs
}

func overlayModelQuota(rows []llmbudget.QuotaRow, now time.Time) error {
	cat := fleet.New()
	agents, errs := cat.Agents()
	if len(errs) > 0 {
		return fmt.Errorf("fleet agents: %v", errs)
	}
	models, errs := cat.Models()
	if len(errs) > 0 {
		return fmt.Errorf("fleet models: %v", errs)
	}
	byName := map[string]fleet.Model{}
	for _, m := range models {
		for _, name := range m.Names() {
			byName[name] = m
		}
	}
	return overlayQuotaSeats(rows, agents, func(name string) (fleet.Model, bool) { m, ok := byName[name]; return m, ok }, quotaQueueDirs(), weave.SeatQuotaStatus, now)
}

func overlayQuotaSeats(rows []llmbudget.QuotaRow, agents []fleet.Agent, model func(string) (fleet.Model, bool), dirs []string, status func([]string, string, string, string, time.Time) (bool, string), now time.Time) error {
	// Resolve each seat only once, even when its provider has several windows.
	type seatState struct {
		status string
		reset  *time.Time
	}
	states := map[string]seatState{}
	for _, r := range rows {
		states[strings.ToLower(r.Provider)] = seatState{}
	}
	for _, a := range agents {
		m, ok := model(a.Model)
		if !ok {
			continue
		}
		provider := strings.ToLower(m.Provider)
		state, wanted := states[provider]
		if !wanted {
			continue
		}
		blocked, reason := status(dirs, a.Tool, a.Model, a.Name, now)
		if !blocked {
			continue
		}
		cause, _, _ := strings.Cut(reason, " ")
		if cause != "cooling-down" && cause != "quota-exhausted" {
			continue
		}
		if state.status != "quota-exhausted" {
			state.status = cause
		}
		if _, raw, ok := strings.Cut(reason, " until "); ok {
			if reset, err := time.Parse(time.RFC3339, raw); err == nil && (state.reset == nil || reset.After(*state.reset)) {
				state.reset = &reset
			}
		}
		states[provider] = state
	}
	for i := range rows {
		r := &rows[i]
		state := states[strings.ToLower(r.Provider)]
		if state.status != "" && r.Status != "quota-exhausted" {
			r.Status = state.status
		}
		if r.ResetAt == nil {
			r.ResetAt = state.reset
		}
	}
	return nil
}
