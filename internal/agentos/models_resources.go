package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/llmbudget"
	"github.com/qiangli/coreutils/pkg/resources"
	"github.com/qiangli/coreutils/pkg/weave"
	"github.com/spf13/cobra"
)

func newModelsResourcesCmd() *cobra.Command {
	cmd := fleet.NewModelsCmd()
	for _, verb := range []string{"usage", "limits", "budget"} {
		child := modelResourcesCommand(verb, llmbudget.CollectReport)
		if verb == "budget" {
			child.AddCommand(modelBudgetConfigureCommand(llmbudget.ConfigurePolicy))
		}
		cmd.AddCommand(child)
	}
	return cmd
}
func modelResourcesCommand(verb string, collect func(context.Context, llmbudget.ReportOptions) (*llmbudget.Report, error)) *cobra.Command {
	var opt sprintMonitorOptions
	var all, jsonOut bool
	cmd := &cobra.Command{Use: verb, Short: map[string]string{"usage": "Observe model usage and spend across shared accounts", "limits": "Inspect known account limits, resets and unavailable sources", "budget": "Inspect configured budgets and outstanding reservations"}[verb], Args: cobra.NoArgs}
	bindMonitorFilters(cmd, &opt)
	cmd.Flags().BoolVar(&all, "all", false, "include every configured and active account (the default)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit versioned account metrics with sources and windows")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if all && (opt.Provider != "" || opt.Model != "") {
			return fmt.Errorf("--all cannot be combined with --provider/--vendor or --model")
		}
		if opt.Host != "" && opt.Host != "local" {
			return fmt.Errorf("model account observations are host-wide; remote target observation requires an authorized sprint monitor transport")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		options := llmbudget.ReportOptions{Provider: opt.Provider, Model: opt.Model, Refresh: opt.Refresh}
		inv, inventoryErr := weave.ReadSprintInventory(ctx, resources.ResourcesStateDir())
		if inv != nil {
			for _, row := range inv.Workloads {
				options.Active = append(options.Active, llmbudget.Attribution{Agent: row.Agent, Model: row.Model, Run: row.ID, Sprint: row.Sprint, Active: true})
			}
		}
		report, err := collect(ctx, options)
		if err != nil {
			return err
		}
		copyReport := *report
		copyReport.Accounts = append([]llmbudget.AccountReport(nil), report.Accounts...)
		report = &copyReport
		if inventoryErr != nil {
			report.Warnings = append(append([]string(nil), report.Warnings...), "active workload attribution unavailable: "+inventoryErr.Error())
		}
		for i := range report.Accounts {
			a := &report.Accounts[i]
			var selected []llmbudget.Metric
			for _, m := range a.Metrics {
				if modelMetricVisible(verb, m.Name) {
					selected = append(selected, m)
				}
			}
			a.Metrics = selected
		}
		if jsonOut {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
		}
		var text strings.Builder
		for _, a := range report.Accounts {
			fmt.Fprintf(&text, "%s account=%s pool=%s %s [%s]\n", a.Provider, a.Account, a.Pool, a.Lane, a.Status)
			fmt.Fprintf(&text, "  shared models: %s; agents: %s\n", strings.Join(a.Models, ", "), strings.Join(a.Agents, ", "))
			for _, m := range a.Metrics {
				value := "unknown"
				if m.Value != nil {
					value = fmt.Sprintf("%.3g %s", *m.Value, m.Unit)
				}
				fmt.Fprintf(&text, "  %s: %s [%s; %s]", m.Name, value, m.Classification, m.Source)
				if m.WindowStart != nil {
					fmt.Fprintf(&text, " since %s", m.WindowStart.Format(time.RFC3339))
				}
				if m.ResetAt != nil {
					fmt.Fprintf(&text, " resets %s", m.ResetAt.Format(time.RFC3339))
				}
				if m.Limitation != "" {
					fmt.Fprintf(&text, "; %s", m.Limitation)
				}
				fmt.Fprintln(&text)
			}
			for _, limitation := range a.Limitations {
				fmt.Fprintf(&text, "  coverage: %s\n", limitation)
			}
		}
		for _, warning := range report.Warnings {
			fmt.Fprintf(&text, "unavailable: %s\n", warning)
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), text.String())
		return err
	}
	return cmd
}
func modelMetricVisible(verb, name string) bool {
	switch verb {
	case "usage":
		return strings.HasPrefix(name, "usage.") || strings.HasPrefix(name, "billing.")
	case "limits":
		return strings.HasPrefix(name, "quota.") || strings.HasPrefix(name, "budget.") || strings.HasPrefix(name, "limit.")
	case "budget":
		return strings.HasPrefix(name, "budget.") || strings.HasPrefix(name, "billing.")
	}
	return false
}

func modelBudgetConfigureCommand(configure func(context.Context, string, bool) (llmbudget.PolicyChange, error)) *cobra.Command {
	var file string
	var apply, jsonOut bool
	cmd := &cobra.Command{Use: "configure --file <policy.json> [--apply]", Short: "Validate a budget policy; explicitly apply with --apply", Args: cobra.NoArgs}
	cmd.Flags().StringVar(&file, "file", "", "policy JSON file to validate")
	cmd.Flags().BoolVar(&apply, "apply", false, "atomically apply the validated policy")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the validation summary")
	_ = cmd.MarkFlagRequired("file")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		result, err := configure(ctx, file, apply)
		if err != nil {
			return err
		}
		if jsonOut {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		action := "validated (dry run)"
		if result.Applied {
			action = "applied"
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "policy %s: version %d, %d bindings, %d constraints, %d sources; %s\n", action, result.Version, result.Bindings, result.Constraints, result.Sources, result.Path)
		return err
	}
	return cmd
}
