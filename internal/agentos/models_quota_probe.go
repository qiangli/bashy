package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/snippet"
	"github.com/spf13/cobra"
)

type quotaProbe struct {
	ID, Provider, Model, Plan string
	Probe                     fleet.UsageProbe
}
type quotaProbeResolver func(string) ([]quotaProbe, error)

func resolveQuotaProbes(selector string) ([]quotaProbe, error) {
	name, id, hasID := strings.Cut(selector, "/")
	if name == "" || (hasID && id == "") {
		return nil, fmt.Errorf("expected RECORD[/PROBEID]")
	}
	cat := fleet.New()
	plan, planOK := cat.Plan(name)
	model, modelOK := cat.Model(name)
	if planOK && modelOK {
		return nil, fmt.Errorf("ambiguous record %q: both plan and model", name)
	}
	var probes []fleet.UsageProbe
	base := quotaProbe{}
	switch {
	case planOK:
		base.ID = "probe:plan:" + plan.Name
		base.Plan = plan.Name
		probes = plan.Usage
	case modelOK:
		base.ID = "probe:model:" + model.Name
		base.Provider = model.Provider
		base.Model = model.Name
		base.Plan = model.Plan
		probes = model.Usage
	default:
		return nil, fmt.Errorf("record %q not found", name)
	}
	var out []quotaProbe
	for _, p := range probes {
		if !hasID || p.ID == id {
			v := base
			v.ID += "/" + p.ID
			v.Probe = p
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("not-configured: no matching probes for %s", selector)
	}
	return out, nil
}
func quotaEffects(p quotaProbe) string {
	if len(p.Probe.Run.Effects) == 0 {
		return "pure"
	}
	return strings.Join(p.Probe.Run.Effects, ", ")
}
func modelQuotaConsentCommand(verb string, resolve quotaProbeResolver, change func(string) error) *cobra.Command {
	return &cobra.Command{Use: verb + " RECORD/PROBEID", Short: verb + " a declared usage probe", SilenceUsage: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !strings.Contains(args[0], "/") {
			return fmt.Errorf("expected RECORD/PROBEID")
		}
		probes, err := resolve(args[0])
		if err != nil {
			return err
		}
		if len(probes) != 1 {
			return fmt.Errorf("expected exactly one probe")
		}
		p := probes[0]
		if err = change(p.ID); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\neffects: %s\n", verb, p.ID, quotaEffects(p))
		return err
	}}
}
func runQuotaProbe(ctx context.Context, p quotaProbe) (snippet.Result, error) {
	ctx = snippet.WithModel(ctx, p.Provider, p.Model, p.Plan)
	if strings.HasPrefix(p.ID, "probe:plan:") {
		ctx = snippet.WithMeasuring(ctx, p.Plan)
	}
	return snippet.Run(ctx, p.Probe.Run, snippet.IO{})
}
func modelQuotaTestCommand(resolve quotaProbeResolver, run func(context.Context, quotaProbe) (snippet.Result, error), clock func() time.Time) *cobra.Command {
	return &cobra.Command{Use: "test RECORD[/PROBEID]", Short: "Run probes once, uncached, with explicit operator consent", SilenceUsage: true, Args: cobra.ExactArgs(1), Long: `Run each selected probe once, without cache or saved consent. Declared effects are printed before execution.

Fields: effects, exit, stdout (up to 4 KiB), stderr (credential values redacted by snippet runner), mapped metrics, refused commands, jq errors, status.`, RunE: func(cmd *cobra.Command, args []string) error {
		probes, err := resolve(args[0])
		if err != nil {
			return err
		}
		var failures []error
		for _, p := range probes {
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "%s\neffects: %s\n", p.ID, quotaEffects(p)); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			result, runErr := run(ctx, p)
			cancel()
			status := "ok"
			switch {
			case errors.Is(runErr, snippet.ErrNotConfigured):
				status = "not-configured"
			case errors.Is(runErr, snippet.ErrNeedsInput) || result.Exit == 6:
				status = "needs-input"
			case runErr != nil || result.Exit != 0 || len(result.Refused) > 0:
				status = "unavailable"
			}
			var metrics []llmbudget.Metric
			var mapErr error
			if status == "ok" {
				metrics, mapErr = mapQuotaProbe(p, result, clock())
				if mapErr != nil {
					status = "unavailable"
				}
			}
			stdout := result.Stdout
			truncated := ""
			if len(stdout) > 4096 {
				stdout = stdout[:4096]
				truncated = "\n[truncated at 4 KiB]"
			}
			fmt.Fprintf(out, "exit: %d\nstdout:\n%s%s\nstderr:\n%s\n", result.Exit, stdout, truncated, result.Stderr)
			fmt.Fprint(out, "mapped metrics: ")
			if metrics == nil {
				metrics = []llmbudget.Metric{}
			}
			if err := json.NewEncoder(out).Encode(metrics); err != nil {
				return err
			}
			fmt.Fprintf(out, "refused commands: %s\njq errors: %s\nstatus: %s\n", quotaCell(strings.Join(result.Refused, ", ")), quotaError(mapErr), status)
			if runErr != nil {
				fmt.Fprintf(out, "error: %v\n", runErr)
			}
			if status != "ok" {
				failures = append(failures, fmt.Errorf("%s: %s", p.ID, status))
			}
		}
		return errors.Join(failures...)
	}}
}
func quotaError(err error) string {
	if err == nil {
		return "-"
	}
	return err.Error()
}

// Diagnostic mapping mirrors the probe adapter's wire vocabulary. The adapter's
// mapping is private; running it again would execute operator effects twice.
func mapQuotaProbe(p quotaProbe, r snippet.Result, now time.Time) ([]llmbudget.Metric, error) {
	var doc any
	if err := json.Unmarshal(r.Stdout, &doc); err != nil {
		return nil, fmt.Errorf("probe JSON: %w", err)
	}
	classification := r.Classification
	if classification == "" {
		classification = "measured"
		if snippet.IsAgentic(p.Probe.Run) {
			classification = "inferred"
		}
	}
	var out []llmbudget.Metric
	add := func(name string, v any, unit, window string, reset any) error {
		n, err := strconv.ParseFloat(fmt.Sprint(v), 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf("probe metric %s is not numeric", name)
		}
		metricName := "usage." + name
		if name == "used_percent" {
			metricName = "quota.used_percent"
		}
		m := llmbudget.Metric{Name: metricName, Value: &n, Unit: unit, Window: window, Classification: classification, Source: p.ID, ObservedAt: now}
		if reset != nil {
			t, e := time.Parse(time.RFC3339, fmt.Sprint(reset))
			if e != nil {
				if f, e := strconv.ParseFloat(fmt.Sprint(reset), 64); e == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
					if f > 1e12 {
						f /= 1000
					}
					t = time.Unix(int64(f), 0).UTC()
				}
			}
			if !t.IsZero() {
				m.ResetAt = &t
				m.WindowEnd = &t
			}
		}
		out = append(out, m)
		return nil
	}
	if len(p.Probe.Metrics) == 0 {
		var wire struct {
			Schema  string `json:"schema_version"`
			Metrics []struct {
				Name   string `json:"name"`
				Value  any    `json:"value"`
				Unit   string `json:"unit"`
				Window string `json:"window"`
				Reset  any    `json:"reset_at"`
			} `json:"metrics"`
		}
		if err := json.Unmarshal(r.Stdout, &wire); err != nil {
			return nil, err
		}
		if wire.Schema != "bashy-usage-v1" || wire.Metrics == nil {
			return nil, fmt.Errorf("expected bashy-usage-v1 with metrics")
		}
		for _, m := range wire.Metrics {
			if err := add(m.Name, m.Value, m.Unit, m.Window, m.Reset); err != nil {
				return out, err
			}
		}
		return out, nil
	}
	for _, m := range p.Probe.Metrics {
		v, err := quotaJQ(m.Value, doc)
		if err != nil {
			return out, err
		}
		unit := m.Unit
		if strings.HasPrefix(unit, ".") {
			u, err := quotaJQ(unit, doc)
			if err != nil {
				return out, err
			}
			unit = fmt.Sprint(u)
		}
		var reset any
		if m.Reset != "" {
			reset, err = quotaJQ(m.Reset, doc)
			if err != nil {
				return out, err
			}
		}
		if err = add(m.Name, v, unit, m.Window, reset); err != nil {
			return out, err
		}
	}
	return out, nil
}
func quotaJQ(expr string, doc any) (any, error) {
	q, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("jq %q: %w", expr, err)
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return nil, fmt.Errorf("jq %q: %w", expr, err)
	}
	v, ok := code.Run(doc).Next()
	if !ok {
		return nil, nil
	}
	if err, ok := v.(error); ok {
		return nil, fmt.Errorf("jq %q: %w", expr, err)
	}
	return v, nil
}
