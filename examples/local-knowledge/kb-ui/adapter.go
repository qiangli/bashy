// Copyright (c) 2026 Qiang Li. All rights reserved.
// Ported from kg-ui under explicit operator authorization for Bashy.
// See ATTRIBUTION.md and LICENSE for licensing information.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxCommandOutputBytes = 5 * 1024 * 1024 // 5 MB max command stdout/stderr
	defaultCommandTimeout = 15 * time.Second
)

// SearchHit represents a single document hit for the explorer/search UI.
type SearchHit struct {
	ID        string  `json:"id"`
	NodeID    string  `json:"node_id,omitempty"`
	Label     string  `json:"label"`
	Type      string  `json:"type"`
	NodeType  string  `json:"node_type"`
	SourceURI string  `json:"source_uri"`
	Score     float64 `json:"score"`
	Artifact  string  `json:"artifact,omitempty"`
	Snippet   string  `json:"snippet,omitempty"`
	Body      string  `json:"body,omitempty"`
}

// SearchResponse is returned by /api/search.
type SearchResponse struct {
	Hits      []SearchHit `json:"hits"`
	ElapsedMs int64       `json:"elapsed_ms"`
	Mode      string      `json:"mode,omitempty"`
	Count     int         `json:"count"`
}

// IndexStatus represents the status of the local search index.
type IndexStatus struct {
	State         string       `json:"state"` // "ready", "indexing", "paused", "empty"
	Paused        bool         `json:"paused"`
	Since         string       `json:"since,omitempty"`
	DocumentCount int          `json:"document_count"`
	Roots         []RootConfig `json:"roots"`
	DiskUsage     int64        `json:"disk_usage_bytes"`
	LastSync      string       `json:"last_sync,omitempty"`
	Error         string       `json:"error,omitempty"`
}

// Adapter defines the contract for local-knowledge search and index operations.
type Adapter interface {
	QueryIndexed(ctx context.Context, query string, mode string, corpus string) (*SearchResponse, error)
	IndexStatus(ctx context.Context) (*IndexStatus, error)
	IndexAdd(ctx context.Context, root string) error
	IndexForget(ctx context.Context, root string) error
	IndexPause(ctx context.Context) error
	IndexResume(ctx context.Context) error
	IndexSync(ctx context.Context) error
	ReadSource(context.Context, string) (SourceText, error)
	GetRootsManager() *RootsManager
}

// ExecCommandBounded executes a command with a bounded context and maximum output buffer size.
type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxCommandOutputBytes {
		return 0, fmt.Errorf("command output exceeds limit")
	}
	return b.Buffer.Write(p)
}
func ExecCommandBounded(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr boundedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("%s: %w (%s)", name, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// ExecSearchIndexed executes `bashy search --indexed QUERY --json`.
func ExecSearchIndexed(ctx context.Context, bashyBin string, query string, extraArgs ...string) ([]byte, error) {
	args := append([]string{"search", "--indexed", query, "--json"}, extraArgs...)
	return ExecCommandBounded(ctx, bashyBin, args...)
}

// ExecSearchIndexStatus executes `bashy search index status --json`.
func ExecSearchIndexStatus(ctx context.Context, bashyBin string) ([]byte, error) {
	return ExecCommandBounded(ctx, bashyBin, "search", "index", "status", "--json")
}

// ExecSearchIndexAdd executes `bashy search index add ROOT`.
func ExecSearchIndexAdd(ctx context.Context, bashyBin string, root string) error {
	_, err := ExecCommandBounded(ctx, bashyBin, "search", "index", "add", root)
	return err
}

// ExecSearchIndexForget executes `bashy search index forget ROOT`.
func ExecSearchIndexForget(ctx context.Context, bashyBin string, root string) error {
	_, err := ExecCommandBounded(ctx, bashyBin, "search", "index", "forget", root)
	return err
}

// ExecSearchIndexPause executes `bashy search index pause`.
func ExecSearchIndexPause(ctx context.Context, bashyBin string) error {
	_, err := ExecCommandBounded(ctx, bashyBin, "search", "index", "pause")
	return err
}

// ExecSearchIndexResume executes `bashy search index resume`.
func ExecSearchIndexResume(ctx context.Context, bashyBin string) error {
	_, err := ExecCommandBounded(ctx, bashyBin, "search", "index", "resume")
	return err
}

// ExecSearchIndexSync executes `bashy search index sync`.
func ExecSearchIndexSync(ctx context.Context, bashyBin string) error {
	_, err := ExecCommandBounded(ctx, bashyBin, "search", "index", "sync")
	return err
}

// ParseSearchEnvelope tolerantly parses JSON search envelopes from documented real shapes:
//  1. Shared kbmemory retrieval envelope (`bashy-kb-memory-retrieval-v1`)
//  2. bashy-search envelope (`bashy-search-v1`)
//  3. Direct hits list (`{"hits": [...]}`)
func ParseSearchEnvelope(data []byte) (*SearchResponse, error) {
	if len(data) == 0 {
		return &SearchResponse{Hits: []SearchHit{}, Count: 0}, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode search response json: %w", err)
	}

	if ok, exists := raw["ok"].(bool); exists && !ok {
		return nil, fmt.Errorf("shared retrieval failed: %v", raw["error"])
	}
	schemaVer, _ := raw["schema_version"].(string)

	// Shape 1: Shared kbmemory envelope (`bashy-kb-memory-retrieval-v1`)
	if schemaVer == "bashy-kb-memory-retrieval-v1" {
		res, _ := raw["result"].(map[string]any)
		mode, _ := res["mode"].(string)
		hitsRaw, _ := res["hits"].([]any)

		hits := make([]SearchHit, 0, len(hitsRaw))
		for _, h := range hitsRaw {
			m, ok := h.(map[string]any)
			if !ok {
				continue
			}
			ref, _ := m["ref"].(string)
			score, _ := m["score"].(float64)
			body, _ := m["body"].(string)
			uri, _ := m["uri"].(string)
			if rec, ok := m["record"].(map[string]any); ok {
				body, _ = rec["body"].(string)
				if source, ok := rec["source"].(map[string]any); ok {
					uri, _ = source["ref"].(string)
				}
			}

			if uri == "" && strings.HasPrefix(ref, "file://") {
				uri = ref
			}

			label := filepath.Base(uri)
			if label == "." || label == "/" || label == "" {
				label = ref
			}

			hit := SearchHit{
				ID:        ref,
				NodeID:    ref,
				Label:     label,
				Type:      "Document",
				NodeType:  "Document",
				SourceURI: uri,
				Score:     score,
				Body:      body,
			}
			hits = append(hits, hit)
		}
		return &SearchResponse{
			Hits:  hits,
			Count: len(hits),
			Mode:  mode,
		}, nil
	}

	// Shape 2: bashy-search envelope (`bashy-search-v1`)
	if schemaVer == "bashy-search-v1" {
		resultsRaw, _ := raw["results"].([]any)
		hits := make([]SearchHit, 0, len(resultsRaw))
		for _, r := range resultsRaw {
			m, ok := r.(map[string]any)
			if !ok {
				continue
			}
			urlStr, _ := m["url"].(string)
			title, _ := m["title"].(string)
			snippet, _ := m["snippet"].(string)

			id := urlStr
			if id == "" {
				id = title
			}

			hit := SearchHit{
				ID:        id,
				NodeID:    id,
				Label:     title,
				Type:      "Document",
				NodeType:  "Document",
				SourceURI: urlStr,
				Score:     1.0,
				Snippet:   snippet,
			}
			hits = append(hits, hit)
		}
		return &SearchResponse{
			Hits:  hits,
			Count: len(hits),
			Mode:  "web",
		}, nil
	}

	// Shape 3: Direct hits array
	if hitsRaw, ok := raw["hits"].([]any); ok {
		hits := make([]SearchHit, 0, len(hitsRaw))
		for _, h := range hitsRaw {
			m, ok := h.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			if id == "" {
				id, _ = m["node_id"].(string)
			}
			label, _ := m["label"].(string)
			nodeType, _ := m["type"].(string)
			if nodeType == "" {
				nodeType, _ = m["node_type"].(string)
			}
			if nodeType == "" {
				nodeType = "Document"
			}
			uri, _ := m["source_uri"].(string)
			score, _ := m["score"].(float64)
			artifact, _ := m["artifact"].(string)
			snippet, _ := m["snippet"].(string)
			body, _ := m["body"].(string)

			hits = append(hits, SearchHit{
				ID:        id,
				NodeID:    id,
				Label:     label,
				Type:      nodeType,
				NodeType:  nodeType,
				SourceURI: uri,
				Score:     score,
				Artifact:  artifact,
				Snippet:   snippet,
				Body:      body,
			})
		}
		return &SearchResponse{
			Hits:  hits,
			Count: len(hits),
		}, nil
	}

	return &SearchResponse{Hits: []SearchHit{}, Count: 0}, nil
}

// ParseSearchIndexStatus parses the JSON status output from `search index status --json`.
func ParseSearchIndexStatus(data []byte) (*IndexStatus, error) {
	var raw struct {
		Schema    string `json:"schema_version"`
		Enabled   bool   `json:"enabled"`
		Paused    bool   `json:"paused"`
		Phase     string `json:"phase"`
		Documents int    `json:"documents"`
		LastSync  string `json:"last_sync"`
		LastError string `json:"last_error"`
		Roots     []struct {
			Path       string    `json:"path"`
			State      string    `json:"state"`
			Reason     string    `json:"reason"`
			Documents  int       `json:"documents"`
			LastUpdate time.Time `json:"last_update"`
			Error      string    `json:"last_error"`
		} `json:"roots"`
	}
	if e := json.Unmarshal(data, &raw); e != nil {
		return nil, e
	}
	if raw.Schema != "bashy-index-v1" {
		return nil, fmt.Errorf("unsupported index status schema %q", raw.Schema)
	}
	state := "ready"
	if raw.Phase != "" && raw.Phase != "idle" {
		state = "indexing"
	}
	if raw.Paused || !raw.Enabled {
		state = "paused"
	}
	if raw.LastError != "" {
		state = "error"
	}
	st := &IndexStatus{State: state, Paused: raw.Paused || !raw.Enabled, DocumentCount: raw.Documents, LastSync: raw.LastSync, Error: raw.LastError, Roots: []RootConfig{}}
	for _, r := range raw.Roots {
		status := r.State
		if status == "watching" {
			status = "active"
		}
		st.Roots = append(st.Roots, RootConfig{Path: r.Path, Resolved: r.Path, Status: status, Includes: []string{r.Reason}, Excludes: []string{"shared index policy: secrets, ignored files, caches, builds, hidden application state, duplicate worktrees"}, FileCount: r.Documents, UpdatedAt: r.LastUpdate, Error: r.Error})
	}
	return st, nil
}

// The adapter only translates the public shared-index CLI. It never scans files
// or treats a backend failure as a successful empty/healthy index.
type BashyCLIAdapter struct {
	bashyBin string
	rootsMgr *RootsManager
	run      func(context.Context, ...string) ([]byte, error)
}

func NewBashyCLIAdapter(bin string, roots *RootsManager) *BashyCLIAdapter {
	if bin == "" {
		bin = "bashy"
	}
	a := &BashyCLIAdapter{bashyBin: bin, rootsMgr: roots}
	a.run = func(ctx context.Context, args ...string) ([]byte, error) {
		return ExecCommandBounded(ctx, bin, args...)
	}
	return a
}
func (a *BashyCLIAdapter) GetRootsManager() *RootsManager { return a.rootsMgr }
func (a *BashyCLIAdapter) call(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()
	return a.run(ctx, args...)
}
func (a *BashyCLIAdapter) QueryIndexed(ctx context.Context, query, mode, corpus string) (*SearchResponse, error) {
	if mode == "" {
		mode = "auto"
	}
	if corpus != "" {
		return nil, fmt.Errorf("corpus selection is unavailable in this adapter")
	}
	start := time.Now()
	out, e := a.call(ctx, "search", "--indexed", query, "--mode", mode, "--json")
	if e != nil {
		return nil, e
	}
	r, e := ParseSearchEnvelope(out)
	if e != nil {
		return nil, e
	}
	r.ElapsedMs = time.Since(start).Milliseconds()
	return r, nil
}
func (a *BashyCLIAdapter) IndexStatus(ctx context.Context) (*IndexStatus, error) {
	out, e := a.call(ctx, "search", "index", "status", "--json")
	if e != nil {
		return nil, e
	}
	st, e := ParseSearchIndexStatus(out)
	if e != nil {
		return nil, e
	}
	a.rootsMgr.Replace(st.Roots)
	return st, nil
}
func (a *BashyCLIAdapter) IndexAdd(ctx context.Context, path string) error {
	root, e := ResolvePath(path)
	if e != nil {
		return e
	}
	if _, e = a.call(ctx, "search", "index", "init", "--json"); e != nil {
		return e
	}
	_, e = a.call(ctx, "search", "index", "add", root, "--json")
	if e != nil {
		return e
	}
	_, e = a.IndexStatus(ctx)
	return e
}
func (a *BashyCLIAdapter) IndexForget(ctx context.Context, path string) error {
	root, e := ResolvePath(path)
	if e != nil {
		return e
	}
	_, e = a.call(ctx, "search", "index", "forget", root, "--json")
	if e != nil {
		return e
	}
	_, e = a.IndexStatus(ctx)
	return e
}
func (a *BashyCLIAdapter) IndexPause(ctx context.Context) error {
	_, e := a.call(ctx, "search", "index", "pause", "--json")
	return e
}
func (a *BashyCLIAdapter) IndexResume(ctx context.Context) error {
	_, e := a.call(ctx, "search", "index", "resume", "--json")
	return e
}
func (a *BashyCLIAdapter) IndexSync(ctx context.Context) error {
	_, e := a.call(ctx, "search", "index", "sync", "--json")
	return e
}

type SourceText struct {
	ID      string `json:"id"`
	Updated string `json:"updated"`
	Body    string `json:"body"`
	Title   string `json:"title"`
	Source  struct {
		Ref string `json:"ref"`
	} `json:"source"`
}

func (a *BashyCLIAdapter) ReadSource(ctx context.Context, uri string) (SourceText, error) {
	if filepath.IsAbs(uri) {
		uri = fileURI(uri)
	}
	out, e := a.call(ctx, "search", "index", "source", uri, "--json")
	if e != nil {
		return SourceText{}, e
	}
	var r SourceText
	e = json.Unmarshal(out, &r)
	return r, e
}
func fileURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
