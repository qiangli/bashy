package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/quotaobs"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

func modelQuotaLogCommand(log func(context.Context, llmbudget.LogFilter) []llmbudget.LogEntry, clock func() time.Time) *cobra.Command {
	var account, since string
	var limit int
	var jsonOut bool
	cmd := &cobra.Command{Use: "log", Short: "Show recorded vendor quota observations", Args: cobra.NoArgs, SilenceUsage: true}
	cmd.Flags().StringVar(&account, "account", "", "filter by account")
	cmd.Flags().StringVar(&since, "since", "24h", "lookback duration")
	cmd.Flags().IntVar(&limit, "limit", 50, "maximum entries")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit full entries including excerpts")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		d, err := time.ParseDuration(since)
		if err != nil || d < 0 {
			return fmt.Errorf("invalid --since duration %q", since)
		}
		if limit < 0 {
			return fmt.Errorf("--limit must be nonnegative")
		}
		entries := log(cmd.Context(), llmbudget.LogFilter{Account: account, Since: clock().Add(-d), Limit: limit})
		if jsonOut {
			if entries == nil {
				entries = []llmbudget.LogEntry{}
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(entries)
		}
		return renderQuotaLog(cmd.OutOrStdout(), entries, time.Local)
	}
	return cmd
}

func renderQuotaLog(out io.Writer, entries []llmbudget.LogEntry, loc *time.Location) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TIME\tSOURCE\tACCOUNT\tTOOL\tKIND\tWINDOW\tRESET\tCLASSIFICATION\tSUMMARY")
	for _, e := range entries {
		kind, window, reset := "-", "-", "-"
		if e.Parsed != nil {
			kind, window = quotaCell(e.Parsed.Kind), quotaCell(e.Parsed.Window)
			if e.Parsed.ResetAt != nil {
				reset = e.Parsed.ResetAt.In(loc).Format("2006-01-02 15:04")
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", e.At.In(loc).Format("2006-01-02 15:04:05"), quotaCell(e.Source), quotaCell(e.Account), quotaCell(e.Tool), kind, window, reset, quotaCell(e.Classification), quotaCell(e.Analysis))
	}
	return w.Flush()
}

func modelQuotaAnalyzeCommand(analyze func(context.Context, llmbudget.AnalyzeOptions) (llmbudget.AnalyzeResult, error), log func(context.Context, llmbudget.LogFilter) []llmbudget.LogEntry, clock func() time.Time) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "analyze", Short: "Analyze vendor quota observations", Args: cobra.NoArgs, SilenceUsage: true}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show observation counts without running the analyzer")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if dryRun {
			entries := log(cmd.Context(), llmbudget.LogFilter{})
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run: %d observations; no analysis run\n", len(entries))
			return err
		}
		result, err := analyze(cmd.Context(), llmbudget.AnalyzeOptions{Now: clock()})
		if err != nil {
			return err
		}
		if result.Hint != "" {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "hint: "+result.Hint)
			return err
		}
		counts := map[string]int{}
		for _, e := range log(cmd.Context(), llmbudget.LogFilter{}) {
			counts[e.Classification]++
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "analyzed: %d  cached: %d  agree: %d  unconfirmed: %d  inferred: %d  conflict: %d\n", result.Analyzed, result.Cached, counts["observed"], counts["observed-unconfirmed"], counts["inferred"], counts["conflict"])
		return err
	}
	return cmd
}

func modelQuotaForecastCommand(forecast func(context.Context, time.Time) []llmbudget.AccountForecast, clock func() time.Time) *cobra.Command {
	var account string
	var jsonOut bool
	cmd := &cobra.Command{Use: "forecast", Short: "Forecast quota exhaustion from usage history", Args: cobra.NoArgs, SilenceUsage: true}
	cmd.Flags().StringVar(&account, "account", "", "filter by account")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit full forecast entries")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		rows := make([]llmbudget.AccountForecast, 0)
		for _, row := range forecast(cmd.Context(), clock()) {
			if account == "" || row.Account == account {
				rows = append(rows, row)
			}
		}
		if jsonOut {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
		}
		return renderQuotaForecast(cmd.OutOrStdout(), rows, time.Local)
	}
	return cmd
}

func quotaTime(t *time.Time, loc *time.Location) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.In(loc).Format("2006-01-02 15:04")
}
func quotaNumber(n float64) string {
	if n == 0 {
		return "-"
	}
	return strconv.FormatFloat(n, 'f', 1, 64)
}
func renderQuotaForecast(out io.Writer, rows []llmbudget.AccountForecast, loc *time.Location) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ACCOUNT\tWINDOW\tUSED\tTOKENS/%\tCV\tBURN RATE\tP50\tP90\tRESET\tVERDICT\tCONFIDENCE\tREGIME")
	for _, r := range rows {
		reset := "-"
		if !r.ResetAt.IsZero() {
			reset = r.ResetAt.In(loc).Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%.0f%%\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%.2f\t%s\n", quotaCell(r.Account), quotaCell(r.Window), r.UsedPercent, quotaNumber(r.TokensPerPercent), quotaNumber(r.CoefficientOfVariation), quotaNumber(r.BurnRate), quotaTime(r.P50, loc), quotaTime(r.P90, loc), reset, quotaCell(r.Verdict), r.Confidence, quotaCell(r.Regime))
	}
	return w.Flush()
}

func modelQuotaIngestCommand(ingest func(context.Context, time.Time) error, clock func() time.Time) *cobra.Command {
	var logsSince string
	cmd := &cobra.Command{Use: "ingest", Short: "Pull quota samples and weave logs into quota history", Args: cobra.NoArgs, SilenceUsage: true, RunE: func(cmd *cobra.Command, _ []string) error {
		d, err := quotaLogsDuration(logsSince)
		if err != nil || d < 0 {
			return fmt.Errorf("invalid --logs-since duration %q", logsSince)
		}
		now := clock()
		counts, err := ingestQuotaWeaveLogs(cmd.Context(), quotaQueueDirs(), now.Add(-d), now)
		if err != nil {
			return err
		}
		if err := ingest(cmd.Context(), now); err != nil {
			return err
		}
		if _, err := io.WriteString(cmd.OutOrStdout(), "quota history ingested\n"); err != nil {
			return err
		}
		accounts := make([]string, 0, len(counts))
		for account := range counts {
			accounts = append(accounts, account)
		}
		sort.Strings(accounts)
		for _, account := range accounts {
			sources := make([]string, 0, len(counts[account]))
			for source := range counts[account] {
				sources = append(sources, source)
			}
			sort.Strings(sources)
			for _, source := range sources {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %d\n", account, source, counts[account][source]); err != nil {
					return err
				}
			}
		}
		return nil
	}}
	cmd.Flags().StringVar(&logsSince, "logs-since", "30d", "scan weave PTY logs modified within this duration")
	return cmd
}

func quotaLogsDuration(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.ParseInt(days, 10, 64)
		if err != nil || n < 0 || n > int64((1<<63-1)/int64(24*time.Hour)) {
			return 0, fmt.Errorf("invalid duration")
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

type quotaWeaveQueue struct {
	Items []struct {
		ID         int64  `json:"id"`
		Tool       string `json:"tool"`
		Owner      string `json:"owner"`
		LaunchSpec *struct {
			Agent string `json:"agent"`
			Model string `json:"model"`
		} `json:"launch_spec"`
	} `json:"items"`
}

// Scan only the conventional PTY logs for recorded runs. The queue supplies
// the tool and model even when the log has no structured metadata of its own.
func ingestQuotaWeaveLogs(ctx context.Context, dirs []string, since, now time.Time) (map[string]map[string]int, error) {
	counts := map[string]map[string]int{}
	cat := fleet.New()
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(dir, "queue.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var queue quotaWeaveQueue
		if err := json.Unmarshal(data, &queue); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Join(dir, "queue.json"), err)
		}
		for _, run := range queue.Items {
			if run.ID <= 0 {
				continue
			}
			path := filepath.Join(dir, "logs", fmt.Sprintf("issue-%d.log", run.ID))
			stat, err := os.Stat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if !stat.Mode().IsRegular() || stat.ModTime().Before(since) || stat.ModTime().After(now) {
				continue
			}
			tail, err := quotaLogTail(path, 16*1024)
			if err != nil {
				return nil, err
			}
			model := ""
			if run.LaunchSpec != nil {
				model = run.LaunchSpec.Model
			}
			for _, o := range weave.ObservationsFromLog(run.Tool, model, tail, stat.ModTime()) {
				o.Agent = run.Owner
				modelName := model
				if run.LaunchSpec != nil && run.LaunchSpec.Agent != "" {
					if a, ok := cat.Agent(run.LaunchSpec.Agent); ok {
						modelName = a.Model
					}
				}
				if m, ok := cat.Model(modelName); ok {
					o.Plan = m.Plan
					o.Account = quotaModelAccount(m)
				}
				if o.Account == "" {
					o.Account = o.Plan
				}
				if o.Account == "" {
					o.Account = "unresolved"
				}
				if err := quotaobs.Record(ctx, o); err != nil {
					return nil, err
				}
				if counts[o.Account] == nil {
					counts[o.Account] = map[string]int{}
				}
				counts[o.Account][o.Source]++
			}
		}
	}
	return counts, nil
}

func quotaLogTail(path string, limit int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return "", err
	}
	start := stat.Size() - limit
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	b, err := io.ReadAll(io.LimitReader(f, limit))
	return string(b), err
}
