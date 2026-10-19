// Copyright (c) 2026 Qiang Li. All rights reserved.
// Ported from kg-ui under explicit operator authorization for Bashy.
// See ATTRIBUTION.md and LICENSE for licensing information.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/qiangli/yoke/pkg/search"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSearchEnvelopeKBMemory(t *testing.T) {
	kbMemoryJSON := []byte(`{
		"schema_version": "bashy-kb-memory-retrieval-v1",
		"result": {
			"mode": "lexical",
			"freshness": { "stale": false },
			"hits": [
				{
					"ref": "file:///tmp/docs/wheat.md",
					"score": 2.45,
					"mode": "lexical",
					"body": "wheat harvest ledger",
					"uri": "file:///tmp/docs/wheat.md"
				}
			]
		}
	}`)

	resp, err := ParseSearchEnvelope(kbMemoryJSON)
	if err != nil {
		t.Fatalf("ParseSearchEnvelope: %v", err)
	}
	if resp.Count != 1 || len(resp.Hits) != 1 {
		t.Fatalf("want 1 hit, got %d", resp.Count)
	}
	hit := resp.Hits[0]
	if hit.Label != "wheat.md" {
		t.Errorf("want label wheat.md, got %q", hit.Label)
	}
	if hit.SourceURI != "file:///tmp/docs/wheat.md" {
		t.Errorf("want source_uri file:///tmp/docs/wheat.md, got %q", hit.SourceURI)
	}
	if hit.Score != 2.45 {
		t.Errorf("want score 2.45, got %f", hit.Score)
	}
}

func TestParseSearchEnvelopeBashySearchV1(t *testing.T) {
	searchV1JSON := []byte(`{
		"schema_version": "bashy-search-v1",
		"query": "documentation",
		"backend": "brave",
		"count": 1,
		"results": [
			{
				"title": "Bashy Architecture",
				"url": "file:///tmp/docs/arch.md",
				"snippet": "Overview of Bashy layers"
			}
		]
	}`)

	resp, err := ParseSearchEnvelope(searchV1JSON)
	if err != nil {
		t.Fatalf("ParseSearchEnvelope: %v", err)
	}
	if resp.Count != 1 || len(resp.Hits) != 1 {
		t.Fatalf("want 1 hit, got %d", resp.Count)
	}
	hit := resp.Hits[0]
	if hit.Label != "Bashy Architecture" {
		t.Errorf("want label 'Bashy Architecture', got %q", hit.Label)
	}
	if hit.SourceURI != "file:///tmp/docs/arch.md" {
		t.Errorf("want url 'file:///tmp/docs/arch.md', got %q", hit.SourceURI)
	}
}

func TestParseSearchIndexStatus(t *testing.T) {
	statusJSON := []byte(`{"schema_version":"bashy-index-v1","enabled":true,"paused":false,"phase":"idle","documents":42,"roots":[]}`)

	status, err := ParseSearchIndexStatus(statusJSON)
	if err != nil {
		t.Fatalf("ParseSearchIndexStatus: %v", err)
	}
	if status.State != "ready" || status.DocumentCount != 42 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestAdapterRootsIntegration(t *testing.T) {
	root := t.TempDir()
	docFile := filepath.Join(root, "guide.txt")
	if err := os.WriteFile(docFile, []byte("bashy local knowledge guide"), 0644); err != nil {
		t.Fatal(err)
	}

	rm := NewRootsManager([]string{root})
	adapter := realTestAdapter(t, rm)
	if err := adapter.IndexAdd(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	// 1. Check IndexStatus reflects the file
	status, err := adapter.IndexStatus(context.Background())
	if err != nil {
		t.Fatalf("IndexStatus: %v", err)
	}
	if status.DocumentCount != 1 {
		t.Errorf("want DocumentCount 1, got %d", status.DocumentCount)
	}
	if len(status.Roots) != 1 {
		t.Errorf("want 1 root, got %d", len(status.Roots))
	}

	// 2. Query for "guide"
	resp, err := adapter.QueryIndexed(context.Background(), "guide", "", "")
	if err != nil {
		t.Fatalf("QueryIndexed: %v", err)
	}
	if resp.Count != 1 {
		t.Errorf("want 1 hit for 'guide', got %d", resp.Count)
	}
	if resp.Hits[0].Label != "guide.txt" {
		t.Errorf("want label guide.txt, got %q", resp.Hits[0].Label)
	}

	// 3. Pause / resume
	if err := adapter.IndexPause(context.Background()); err != nil {
		t.Fatalf("IndexPause: %v", err)
	}
	status, _ = adapter.IndexStatus(context.Background())
	if !status.Paused || status.State != "paused" {
		t.Errorf("expected paused status, got %+v", status)
	}

	if err := adapter.IndexResume(context.Background()); err != nil {
		t.Fatalf("IndexResume: %v", err)
	}
	status, _ = adapter.IndexStatus(context.Background())
	if status.Paused || status.State != "ready" {
		t.Errorf("expected ready status, got %+v", status)
	}
}

func realTestAdapter(t *testing.T, roots *RootsManager) *BashyCLIAdapter {
	t.Helper()
	t.Setenv("BASHY_HOME", filepath.Join(t.TempDir(), "index-home"))
	t.Setenv("BASHY_COORD_DIR", filepath.Join(t.TempDir(), "coord"))
	a := NewBashyCLIAdapter("unused-test-binary", roots)
	a.run = func(ctx context.Context, args ...string) ([]byte, error) {
		c := search.NewSearchCmd()
		c.SetContext(ctx)
		c.SetArgs(args[1:])
		var b bytes.Buffer
		c.SetOut(&b)
		c.SetErr(io.Discard)
		e := c.Execute()
		return b.Bytes(), e
	}
	return a
}
func TestNestedRecordProvenance(t *testing.T) {
	b, _ := json.Marshal(map[string]any{"schema_version": "bashy-kb-memory-retrieval-v1", "ok": true, "result": map[string]any{"mode": "lexical", "hits": []any{map[string]any{"ref": "stable-id", "score": 1, "record": map[string]any{"body": "preview", "source": map[string]any{"ref": "file:///allowed/note.md"}}}}}})
	r, e := ParseSearchEnvelope(b)
	if e != nil || len(r.Hits) != 1 || r.Hits[0].SourceURI != "file:///allowed/note.md" || r.Hits[0].Body != "preview" {
		t.Fatal(r, e)
	}
}
