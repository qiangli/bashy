package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/qiangli/yoke/pkg/llmbudget"
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
	return &cobra.Command{Use: "ingest", Short: "Pull quota samples into forecast history", Args: cobra.NoArgs, SilenceUsage: true, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := ingest(cmd.Context(), clock()); err != nil {
			return err
		}
		_, err := io.WriteString(cmd.OutOrStdout(), "quota history ingested\n")
		return err
	}}
}
