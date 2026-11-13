package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/providerselector"
	"github.com/qiangli/yoke/pkg/search"
	"github.com/qiangli/yoke/pkg/telemetry"
	"github.com/spf13/cobra"
)

type searchSelection struct {
	Provider string
	Route    string
	Yes      bool
	Max      int
}

// selectedSearch runs the same catalog selection used by the service door.
// API adapters retain the established cited search result schema; vendor
// commands must return cited results in that schema too.
func selectedSearch(ctx context.Context, query string, opt searchSelection) ([]search.Result, providerselector.Candidate, error) {
	chosen, err := chooseSearchProvider(query, opt)
	if err != nil {
		return nil, providerselector.Candidate{}, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, providerselector.Candidate{}, err
	}
	args := []string{}
	if chosen.Kind == "tool-command" {
		prompt := "Use web search for: " + query + ". Return up to " + strconv.Itoa(opt.Max) + " source titles and their URLs as markdown links. Do not answer from memory."
		args = []string{"tool", "cmd", "run", chosen.Name, "--json", "--", prompt}
	} else {
		args = []string{chosen.Name, "--json", "--max", strconv.Itoa(opt.Max), query}
	}
	command := exec.CommandContext(ctx, self, args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	raw, err := command.Output()
	if err != nil {
		return nil, providerselector.Candidate{}, fmt.Errorf("search provider %s: %w: %s", chosen.Name, err, strings.TrimSpace(stderr.String()))
	}
	results, err := decodeSearchResults(raw, chosen)
	if err != nil {
		return nil, providerselector.Candidate{}, err
	}
	if opt.Max > 0 && len(results) > opt.Max {
		results = results[:opt.Max]
	}
	for _, result := range results {
		if !strings.HasPrefix(result.URL, "https://") && !strings.HasPrefix(result.URL, "http://") {
			return nil, providerselector.Candidate{}, fmt.Errorf("search provider %s returned a result without an HTTP citation", chosen.Name)
		}
		if result.RetrievedAt.IsZero() {
			return nil, providerselector.Candidate{}, fmt.Errorf("search provider %s returned a result without retrieval time", chosen.Name)
		}
	}
	return results, chosen, nil
}

func chooseSearchProvider(query string, opt searchSelection) (providerselector.Candidate, error) {
	tools, errs := fleet.New().Tools(false)
	if len(errs) > 0 {
		return providerselector.Candidate{}, fmt.Errorf("search providers: %w", errs[0])
	}
	selection := providerselector.Options{Role: "search", Provider: opt.Provider, Route: opt.Route, Request: map[string]any{"query": query}, Yes: opt.Yes}
	return providerselector.Select(providerselector.Project(registeredCommands(), tools, selection), selection)
}

// newProviderSearchCmd keeps the local/indexed and deprecated --backend paths
// in yoke's existing search command, and routes web searches through the
// shared service selector. No new yoke module pin is needed for this door.
func newProviderSearchCmd() *cobra.Command {
	cmd := search.NewSearchCmd()
	cmd.Long = "Search local and indexed content, or search the web through registered providers. " +
		"Use --provider to select one; --backend is the deprecated direct API transport override. " +
		"Web results include source URLs and retrieval times."
	cmd.Flags().Lookup("provider").Usage = "web provider name from bashy providers search"
	base := cmd.RunE
	var route string
	var yes bool
	var providers bool
	var dryRun bool
	cmd.Flags().StringVar(&route, "route", "", "web provider route: oss | vendor | browser | api")
	cmd.Flags().BoolVar(&yes, "yes", false, "allow a provider with spend/send effects")
	cmd.Flags().BoolVar(&providers, "providers", false, "list web search providers")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "select a provider without running it")
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if providers {
			listArgs := []string{"search"}
			if jsonOut, _ := c.Flags().GetBool("json"); jsonOut {
				listArgs = append(listArgs, "--json")
			}
			if code := runProviders(listArgs, c.OutOrStdout(), c.ErrOrStderr()); code != 0 {
				return fmt.Errorf("search: provider listing failed (exit %d)", code)
			}
			return nil
		}
		for _, flag := range []string{"local", "content", "files", "kb", "indexed", "index", "index-status", "backend", "mode"} {
			if c.Flags().Changed(flag) {
				return base(c, args)
			}
		}
		if len(args) == 0 {
			return base(c, args)
		}
		// A bare query keeps the key ladder it always had; vendor providers
		// tie on route, so auto-selection among them would refuse it.
		if !c.Flags().Changed("provider") && route == "" && !dryRun {
			return base(c, args)
		}
		provider, _ := c.Flags().GetString("provider")
		max, _ := c.Flags().GetInt("max")
		jsonOut, _ := c.Flags().GetBool("json")
		query := strings.Join(args, " ")
		selection := searchSelection{Provider: provider, Route: route, Yes: yes, Max: max}
		if dryRun {
			chosen, err := chooseSearchProvider(query, selection)
			if err != nil {
				return err
			}
			if jsonOut {
				return json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"schema_version": "bashy-svc-v1", "role": "search", "provider": chosen.Name, "route": chosen.Route, "outcome": "dry-run", "outputs": []any{}, "text": "", "cost_class": chosen.Route, "elapsed": 0})
			}
			fmt.Fprintf(c.OutOrStdout(), "%s\t%s\n", chosen.Name, chosen.Route)
			return nil
		}
		start := time.Now()
		results, used, err := selectedSearch(c.Context(), query, selection)
		if err != nil {
			return err
		}
		telemetry.Provenance(c.Context(), "search.results", int64(len(results)), used.Name)
		if jsonOut {
			return json.NewEncoder(c.OutOrStdout()).Encode(struct {
				SchemaVersion string          `json:"schema_version"`
				Role          string          `json:"role"`
				Provider      string          `json:"provider"`
				Route         string          `json:"route"`
				Outcome       string          `json:"outcome"`
				Outputs       []any           `json:"outputs"`
				Text          string          `json:"text"`
				CostClass     string          `json:"cost_class"`
				Elapsed       float64         `json:"elapsed"`
				Query         string          `json:"query"`
				Backend       string          `json:"backend"`
				Count         int             `json:"count"`
				Results       []search.Result `json:"results"`
			}{"bashy-svc-v1", "search", used.Name, used.Route, "success", []any{}, "", used.Route, time.Since(start).Seconds(), query, used.Name, len(results), results})
		}
		for i, result := range results {
			fmt.Fprintf(c.OutOrStdout(), "%d. %s\n   %s\n", i+1, result.Title, result.URL)
			if result.Snippet != "" {
				fmt.Fprintf(c.OutOrStdout(), "   %s\n", result.Snippet)
			}
		}
		fmt.Fprintf(c.ErrOrStderr(), "(%d results via %s)\n", len(results), used.Name)
		return nil
	}
	return cmd
}

var markdownCitation = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)

func decodeSearchResults(raw []byte, chosen providerselector.Candidate) ([]search.Result, error) {
	if strings.HasPrefix(chosen.Name, "search-codex") || strings.HasPrefix(chosen.Name, "search-agy") || strings.HasPrefix(chosen.Name, "search-muse") {
		return decodeVendorSearchEvents(raw, chosen.Name)
	}
	var envelope struct {
		Outcome string          `json:"outcome"`
		Text    string          `json:"text"`
		Result  string          `json:"result"`
		Results []search.Result `json:"results"`
		Usage   struct {
			ServerToolUse struct {
				WebSearchRequests int `json:"web_search_requests"`
			} `json:"server_tool_use"`
		} `json:"usage"`
		ModelUsage map[string]struct {
			WebSearchRequests int `json:"webSearchRequests"`
		} `json:"modelUsage"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("search provider %s did not return JSON: %w", chosen.Name, err)
	}
	if envelope.Outcome != "" && envelope.Outcome != "success" {
		return nil, fmt.Errorf("search provider %s outcome %s", chosen.Name, envelope.Outcome)
	}
	if chosen.Name == "search-claude" {
		requests := envelope.Usage.ServerToolUse.WebSearchRequests
		for _, usage := range envelope.ModelUsage {
			requests += usage.WebSearchRequests
		}
		if requests == 0 {
			return nil, fmt.Errorf("search provider %s did not use WebSearch", chosen.Name)
		}
	}
	if chosen.Kind == "tool-command" || chosen.Name == "search-claude" {
		body := envelope.Text
		if envelope.Result != "" {
			body = envelope.Result
		}
		matches := markdownCitation.FindAllStringSubmatch(body, -1)
		seen := map[string]bool{}
		for _, m := range matches {
			if seen[m[2]] {
				continue
			}
			seen[m[2]] = true
			envelope.Results = append(envelope.Results, search.Result{Title: m[1], URL: m[2], RetrievedAt: time.Now().UTC(), Backend: chosen.Name})
		}
	}
	if len(envelope.Results) == 0 {
		return nil, fmt.Errorf("search provider %s returned no cited results", chosen.Name)
	}
	return envelope.Results, nil
}

// decodeVendorSearchEvents accepts only an actual search tool invocation and
// cited output. A fluent answer from model memory is not a search result.
func decodeVendorSearchEvents(raw []byte, provider string) ([]search.Result, error) {
	usedSearch := false
	completed := false
	body := ""
	museSearchTask := ""
	var results []search.Result
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event struct {
			Type        string `json:"type"`
			Event       string `json:"event"`
			PayloadType string `json:"payload_type"`
			Item        struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Results []struct {
					Title   string `json:"title"`
					URL     string `json:"url"`
					Snippet string `json:"snippet"`
				} `json:"results"`
			} `json:"item"`
			StepUpdate struct {
				State    string `json:"state"`
				StepType string `json:"step_type"`
				ToolName string `json:"tool_name"`
			} `json:"step_update"`
			Result struct {
				Status   string `json:"status"`
				Response string `json:"response"`
			} `json:"result"`
			Payload struct {
				TaskID   string `json:"task_id"`
				Text     string `json:"text"`
				Terminal string `json:"terminal"`
				Event    struct {
					TaskKind string `json:"task_kind"`
				} `json:"event"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("search provider %s returned malformed event: %w", provider, err)
		}
		switch provider {
		case "search-codex":
			if event.Type == "item.completed" && event.Item.Type == "web_search" {
				usedSearch = true
				for _, hit := range event.Item.Results {
					if hit.Title != "" && hit.URL != "" {
						results = append(results, search.Result{Title: hit.Title, URL: hit.URL, Snippet: hit.Snippet, RetrievedAt: time.Now().UTC(), Backend: provider})
					}
				}
			}
			if event.Type == "turn.completed" {
				completed = true
			}
		case "search-agy":
			if event.Event == "step_update" && event.StepUpdate.StepType == "tool" && event.StepUpdate.ToolName == "search_web" && event.StepUpdate.State == "DONE" {
				usedSearch = true
			}
			if event.Event == "result" && event.Result.Status == "SUCCESS" {
				completed = true
				body = event.Result.Response
			}
		case "search-muse":
			if event.PayloadType == "task.lifecycle.proposed" && event.Payload.Event.TaskKind == "tool.web_search" {
				museSearchTask = event.Payload.TaskID
			}
			if event.PayloadType == "task.lifecycle.completed" && museSearchTask != "" && event.Payload.TaskID == museSearchTask {
				usedSearch = true
			}
			if event.PayloadType == "run.terminal.completed" && event.Payload.Terminal == "completed" {
				completed = true
				body = event.Payload.Text
			}
		}
	}
	if !usedSearch || !completed {
		return nil, fmt.Errorf("search provider %s did not complete a web search", provider)
	}
	if len(results) == 0 {
		seen := map[string]bool{}
		for _, m := range markdownCitation.FindAllStringSubmatch(body, -1) {
			if seen[m[2]] {
				continue
			}
			seen[m[2]] = true
			results = append(results, search.Result{Title: m[1], URL: m[2], RetrievedAt: time.Now().UTC(), Backend: provider})
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("search provider %s returned no cited results", provider)
	}
	return results, nil
}
