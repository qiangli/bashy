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
