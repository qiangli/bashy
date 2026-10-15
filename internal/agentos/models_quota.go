package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

func modelQuotaCommand(collect func(context.Context, llmbudget.QuotaOptions) ([]llmbudget.QuotaRow, error), overlay func([]llmbudget.QuotaRow, time.Time) error, clock func() time.Time) *cobra.Command {
	return modelQuotaCommandWithMetadata(collect, overlay, clock, loadQuotaProbeMetadata)
}

func modelQuotaCommandWithMetadata(collect func(context.Context, llmbudget.QuotaOptions) ([]llmbudget.QuotaRow, error), overlay func([]llmbudget.QuotaRow, time.Time) error, clock func() time.Time, metadata func() ([]quotaProbeMetadata, error)) *cobra.Command {
	var opt llmbudget.QuotaOptions
	var jsonOut bool
	cmd := &cobra.Command{Use: "quota", Short: "View account quota, resets and recorded cooldowns", SilenceUsage: true, Args: cobra.NoArgs,
		Long: `View account quota and recorded seat cooldowns. Only enabled consent-gated probes run.

Fields:
  LABEL: account display label; ACCOUNT: billing account identity; LIMIT: declared plan limit.
  WINDOW: observation window; USED: percentage used; RESETS: local reset time and time remaining.
  TOKENS/BAL: observed tokens or account balance (with its unit).
  SOURCE: observation source; AGE: rounded time since observation; STATUS: availability.
  Unknown cells are '-'. JSON also includes headroom, stale, suggested_workers and classification.`}
	cmd.Flags().BoolVar(&opt.All, "all", false, "include accounts without configured agents")
	cmd.Flags().BoolVar(&opt.Refresh, "refresh", false, "refresh enabled sources (declared probe timeouts, at most 40 seconds)")
	cmd.Flags().StringVar(&opt.Provider, "provider", "", "filter by provider, account or label")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit bashy-registry-list-v1 quota rows")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		probes, err := metadata()
		if err != nil {
			return err
		}
		timeout := 5 * time.Second
		if opt.Refresh {
			timeout = quotaRefreshTimeout(probes)
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
		sortQuotaRows(rows)
		if jsonOut {
			items := make([]quotaJSONRow, 0, len(rows))
			for _, r := range rows {
				items = append(items, quotaJSONRow{r, r.Headroom, r.Stale, r.SuggestedWorkers, r.Classification})
			}
			return fleet.WriteListJSON(cmd.OutOrStdout(), "quota", fleet.BasicView(false, opt.All), items)
		}
		if err := renderModelQuota(cmd.OutOrStdout(), rows, opt.Now, time.Local); err != nil {
			return err
		}
		return renderQuotaHints(cmd.OutOrStdout(), rows, probes)
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
	rows = append([]llmbudget.QuotaRow(nil), rows...)
	sortQuotaRows(rows)
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "LABEL\tACCOUNT\tWINDOW\tUSED\tRESETS\tLIMIT\tTOKENS/BAL\tSOURCE\tAGE\tSTATUS")
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
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", quotaCell(r.Label), quotaCell(r.Account), quotaCell(r.Window), used, reset, quotaCell(r.Limit), quotaCell(quantity), quotaCell(r.Source), age, quotaCell(r.Status))
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
	// Resolve each seat only once, even when its account has several windows.
	type seatState struct {
		status string
		reset  *time.Time
	}
	states := map[string]seatState{}
	for _, r := range rows {
		states[r.Account] = seatState{}
	}
	for _, a := range agents {
		m, ok := model(a.Model)
		if !ok {
			continue
		}
		account := quotaModelAccount(m)
		state, wanted := states[account]
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
		states[account] = state
	}
	for i := range rows {
		r := &rows[i]
		state := states[r.Account]
		if state.status != "" && r.Status != "quota-exhausted" {
			r.Status = state.status
		}
		if r.ResetAt == nil {
			r.ResetAt = state.reset
		}
	}
	return nil
}

func quotaModelAccount(m fleet.Model) string {
	if m.Kind == fleet.ModelKindLocal {
		return "local"
	}
	if m.Plan != "" {
		return m.Plan
	}
	if m.APIKeyRef == "" && len(m.Usage) > 0 {
		return m.Name
	}
	return m.APIKeyRef
}

func sortQuotaRows(rows []llmbudget.QuotaRow) {
	order := map[string]int{"5h": 0, "day": 1, "week": 2, "month": 3, "concurrent": 4, "-": 5, "": 5}
	rank := func(w string) int {
		if n, ok := order[w]; ok {
			return n
		}
		return 6
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Label != b.Label {
			return a.Label < b.Label
		}
		if rank(a.Window) != rank(b.Window) {
			return rank(a.Window) < rank(b.Window)
		}
		if a.Account != b.Account {
			return a.Account < b.Account
		}
		return a.Window < b.Window
	})
}

type quotaProbeMetadata struct {
	account, selector string
	timeout           time.Duration
	net, enabled      bool
}

// Read declarations and consent only; never execute a probe to discover hints.
func loadQuotaProbeMetadata() ([]quotaProbeMetadata, error) {
	cat := fleet.New()
	models, errs := cat.Models()
	if len(errs) > 0 {
		return nil, fmt.Errorf("fleet models: %v", errs)
	}
	plans, errs := cat.Plans()
	if len(errs) > 0 {
		return nil, fmt.Errorf("fleet plans: %v", errs)
	}
	path := os.Getenv("BASHY_LLM_BUDGET_POLICY")
	if path == "" {
		state := os.Getenv("BASHY_LLM_BUDGET_STATE")
		if state == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			state = filepath.Join(home, ".bashy", "llm-budget.json")
		}
		path = filepath.Join(filepath.Dir(state), "llm-budget-policy.json")
	}
	var policy llmbudget.Policy
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(data, &policy); err != nil {
			return nil, err
		}
	}
	allowed := map[string]bool{}
	for _, id := range policy.ProbeConsent {
		allowed[id] = true
	}
	overrides := map[string]bool{}
	for _, source := range policy.Sources {
		overrides[source.ID] = source.Enabled
	}
	byPlan := map[string]fleet.Plan{}
	for _, p := range plans {
		byPlan[p.Name] = p
	}
	var out []quotaProbeMetadata
	seen := map[string]bool{}
	add := func(kind, record, account string, p fleet.UsageProbe) {
		selector := record + "/" + p.ID
		id := "probe:" + kind + ":" + selector
		if seen[id] {
			return
		}
		seen[id] = true
		net := false
		for _, effect := range p.Run.Effects {
			if effect == "net" {
				net = true
			}
		}
		enabled := allowed[id]
		if v, ok := overrides[id]; ok {
			enabled = v
		}
		out = append(out, quotaProbeMetadata{account: account, selector: selector, timeout: p.Run.Timeout, net: net, enabled: enabled})
	}
	for _, m := range models {
		if !m.Active() {
			continue
		}
		for _, p := range m.Usage {
			add("model", m.Name, quotaModelAccount(m), p)
		}
		for _, p := range byPlan[m.Plan].Usage {
			add("plan", m.Plan, m.Plan, p)
		}
	}
	return out, nil
}

func quotaRefreshTimeout(probes []quotaProbeMetadata) time.Duration {
	longest := time.Duration(0)
	for _, p := range probes {
		timeout := p.timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		} // yoke's default probe timeout
		if timeout > longest {
			longest = timeout
		}
	}
	if len(probes) == 0 {
		longest = 10 * time.Second
	}
	if longest >= 35*time.Second {
		return 40 * time.Second
	}
	return longest + 5*time.Second
}

func renderQuotaHints(out io.Writer, rows []llmbudget.QuotaRow, probes []quotaProbeMetadata) error {
	accounts := map[string]bool{}
	hints := map[string]bool{}
	for _, r := range rows {
		accounts[r.Account] = true
		if r.Status != "not-configured" {
			continue
		}
		for _, reason := range r.Limitations {
			if strings.Contains(reason, "no snapshot at ") && strings.Contains(reason, "/quota/claude.json") {
				hints["hint: register bashy model quota capture --to ~/.bashy/quota/claude.json as the Claude statusLine command"] = true
			}
		}
	}
	for _, p := range probes {
		if accounts[p.account] && p.net && !p.enabled {
			hints["hint: bashy model quota enable "+p.selector] = true
		}
	}
	lines := make([]string, 0, len(hints))
	for hint := range hints {
		lines = append(lines, hint)
	}
	sort.Strings(lines)
	for _, hint := range lines {
		if _, err := fmt.Fprintln(out, hint); err != nil {
			return err
		}
	}
	return nil
}
