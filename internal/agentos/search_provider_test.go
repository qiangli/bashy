package agentos

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/providerselector"
)

func TestDecodeSearchProviderRequiresCitations(t *testing.T) {
	provider := providerselector.Candidate{Name: "claude:search", Kind: "tool-command"}
	results, err := decodeSearchResults([]byte(`{"outcome":"success","text":"[Go](https://go.dev/)"}`), provider)
	if err != nil || len(results) != 1 || results[0].URL != "https://go.dev/" || results[0].Backend != "claude:search" {
		t.Fatalf("cited results = %+v, %v", results, err)
	}
	if _, err := decodeSearchResults([]byte(`{"outcome":"success","text":"Go is at go.dev"}`), provider); err == nil {
		t.Fatal("uncited answer was accepted")
	}
	if _, err := decodeSearchResults([]byte(`{"outcome":"error","text":"[Go](https://go.dev/)"}`), provider); err == nil {
		t.Fatal("failed provider was accepted")
	}
	claude := providerselector.Candidate{Name: "search-claude", Kind: "command"}
	if _, err := decodeSearchResults([]byte(`{"result":"[Go](https://go.dev/)","usage":{"server_tool_use":{"web_search_requests":0}}}`), claude); err == nil {
		t.Fatal("answer without a WebSearch call was accepted")
	}
	results, err = decodeSearchResults([]byte(`{"result":"[Go](https://go.dev/)","usage":{"server_tool_use":{"web_search_requests":1}}}`), claude)
	if err != nil || len(results) != 1 || results[0].URL != "https://go.dev/" {
		t.Fatalf("Claude cited results = %+v, %v", results, err)
	}
	results, err = decodeSearchResults([]byte(`{"result":"[Go](https://go.dev/)","usage":{"server_tool_use":{"web_search_requests":0}},"modelUsage":{"opus":{"webSearchRequests":1}}}`), claude)
	if err != nil || len(results) != 1 {
		t.Fatalf("modelUsage cited results = %+v, %v", results, err)
	}
}

func TestSearchDryRunUsesSharedProviderSelector(t *testing.T) {
	ringDir(t)
	writeRecord(t, fleet.Command{Name: "fixture-search", Script: "true", Route: "api", Provides: []string{"search"}, Effects: []string{"read"}})
	cmd := newProviderSearchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--provider", "fixture-search", "--dry-run", "--json", "go"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"provider":"fixture-search"`) || !strings.Contains(out.String(), `"outcome":"dry-run"`) {
		t.Fatalf("dry-run output = %s", out.String())
	}
}

func TestVendorSearchEventsRequireToolAndCitation(t *testing.T) {
	cases := []struct{ name, events string }{
		{"search-codex", `{"type":"item.completed","item":{"type":"web_search","results":[{"title":"Go","url":"https://go.dev/"}]}}
{"type":"turn.completed"}`},
		{"search-agy", `{"event":"step_update","step_update":{"step_type":"tool","tool_name":"search_web","state":"DONE"}}
{"event":"result","result":{"status":"SUCCESS","response":"[Go](https://go.dev/)"}}`},
		{"search-muse", `{"payload_type":"task.lifecycle.proposed","payload":{"task_id":"t1","event":{"task_kind":"tool.web_search"}}}
{"payload_type":"task.lifecycle.completed","payload":{"task_id":"t1"}}
{"payload_type":"run.terminal.completed","payload":{"terminal":"completed","text":"[Go](https://go.dev/)"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := providerselector.Candidate{Name: tc.name}
			results, err := decodeSearchResults([]byte(tc.events), candidate)
			if err != nil || len(results) != 1 || results[0].URL != "https://go.dev/" || results[0].RetrievedAt.IsZero() {
				t.Fatalf("results = %+v, %v", results, err)
			}
			if _, err := decodeSearchResults([]byte(strings.Split(tc.events, "\n")[len(strings.Split(tc.events, "\n"))-1]), candidate); err == nil {
				t.Fatal("answer without search accepted")
			}
		})
	}
}

func TestBareSearchKeepsKeyLadder(t *testing.T) {
	ringDir(t)
	for _, k := range []string{"TAVILY_API_KEY", "BRAVE_API_KEY", "BRAVE_SEARCH_API_KEY", "SERPER_API_KEY", "SEARXNG_URL"} {
		t.Setenv(k, "")
	}
	for _, name := range []string{"fixture-a", "fixture-b"} {
		writeRecord(t, fleet.Command{Name: name, Script: "true", Route: "api", Provides: []string{"search"}, Effects: []string{"read"}})
	}
	cmd := newProviderSearchCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"go"})
	if err := cmd.Execute(); err != nil && strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("bare search went through the provider selector: %v", err)
	}
}
