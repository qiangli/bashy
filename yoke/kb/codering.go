// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

// This file is kb's OWN copy of the code-ring reader (the "code" form of
// `kb context` / `kb ask`). It is duplicated from yoke/cmds/graph on purpose:
// the operator rule is that no non-internal command depends on another, even
// if it means duplicate code. Keep it in step with that source by hand.

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
	gfygraph "github.com/qiangli/gfy/pkg/graph"
	"github.com/qiangli/gfy/pkg/search"
	"github.com/qiangli/yoke/pkg/codegraph"
	"github.com/qiangli/yoke/pkg/kb"
	"github.com/qiangli/yoke/pkg/recall"
	"github.com/qiangli/yoke/pkg/repomap"
)

// cacheRel is a bashy-OWNED cache path (same file the graph verb writes).
const cacheRel = ".agents/bashy/graph.json"

const (
	codeRingDefaultDepth = 1
	codeRingDefaultLimit = 40
)

// CodeRing is the code form: a read-only view over the bashy graph cache plus
// a budgeted repomap. It is a repo reader, not a stored kb ring.
type CodeRing struct {
	RepoRoot string
}

// NewCodeRing returns the code-form reader bashy can inject into kb context.
func NewCodeRing(repoRoot string) recall.Reader {
	return CodeRing{RepoRoot: repoRoot}
}

func (CodeRing) Ring() string    { return recall.RingRepo }
func (CodeRing) Forms() []string { return []string{kb.FormCode} }

func (r CodeRing) Recall(q recall.Query) ([]recall.Hit, error) {
	root := strings.TrimSpace(r.RepoRoot)
	if root == "" {
		root = "."
	}
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   root,
		FS:    tool.NewLocalFS(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: ioDiscard{}, Err: ioDiscard{}},
	}
	gc, err := loadOrBuild(rc, root, false, true)
	if err != nil {
		return nil, fmt.Errorf("code: open %s: %w", filepath.Join(root, cacheRel), err)
	}

	query := strings.TrimSpace(q.Text)
	hits := graphCodeHits(root, gc, query, codeRingDefaultDepth, codeRingDefaultLimit)
	hits = append(hits, repomapCodeHits(root, q)...)
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].ID < hits[j].ID
	})
	return hits, nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func graphCodeHits(root string, gc *codegraph.GraphContext, query string, depth, limit int) []recall.Hit {
	if query == "" {
		query = "code"
	}
	if gc == nil || gc.Graph == nil {
		return nil
	}
	results := search.ScoreNodes(gc.Graph, query)
	if len(results) > 5 {
		results = results[:5]
	}
	out := make([]recall.Hit, 0, len(results))
	for i, result := range results {
		visited, edges := gc.Graph.BFS([]string{result.ID}, depth)
		total := len(visited)
		if limit > 0 && len(visited) > limit {
			visited = visited[:limit]
		}
		nodes, sgEdges := subgraphView(gc.Graph, visited, edges)
		label := nodeLabel(gc.Graph, result.ID)
		out = append(out, recall.Hit{
			Ring: recall.RingRepo, Kind: "code", ID: "code:" + result.ID, Form: kb.FormCode,
			Cue:   "code " + label,
			Gist:  codeGist(gc.Graph.NodeAttrs(result.ID), total, len(sgEdges)),
			Full:  codeSubgraphText(label, nodes, sgEdges),
			Score: float64(len(results)-i) + result.Score,
			Why:   []string{"codegraph", "depth 1", "limit 40"},
			Source: []recall.SourceRef{{
				URI: "file://" + filepath.Join(root, cacheRel),
			}},
		})
	}
	return out
}

func repomapCodeHits(root string, q recall.Query) []recall.Hit {
	opts := repomap.DefaultOptions()
	opts.MaxTokens = max(80, q.Budget/2)
	opts.MaxFiles = codeRingDefaultLimit
	opts.RelevanceQuery = q.Text
	opts.ChatContextFiles = normalizeRepoFiles(root, q.Files)
	rm, err := repomap.Generate(root, opts)
	if err != nil || rm == nil {
		return nil
	}
	out := make([]recall.Hit, 0, len(rm.Entries))
	for i, entry := range rm.Entries {
		if len(entry.Symbols) == 0 {
			continue
		}
		ref := "code:file:" + entry.Path
		out = append(out, recall.Hit{
			Ring: recall.RingRepo, Kind: "code", ID: ref, Form: kb.FormCode,
			Cue:   "code file " + entry.Path,
			Gist:  repomapGist(entry),
			Full:  repomapEntryText(entry),
			Score: 0.5 + entry.Score - float64(i)*0.001,
			Why:   []string{"repomap", "relevance query", "chat files boost"},
			Source: []recall.SourceRef{{
				URI: "file://" + filepath.Join(root, entry.Path),
			}},
		})
	}
	return out
}

func normalizeRepoFiles(root string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if filepath.IsAbs(file) {
			if rel, err := filepath.Rel(root, file); err == nil {
				file = rel
			}
		}
		out = append(out, filepath.ToSlash(file))
	}
	return out
}

func codeGist(attrs map[string]any, nodes, edges int) string {
	kind := edgeStr(attrs, "file_type")
	source := edgeStr(attrs, "source_file")
	var parts []string
	if kind != "" {
		parts = append(parts, kind)
	}
	if source != "" {
		parts = append(parts, source)
	}
	parts = append(parts, fmt.Sprintf("impact view: %d nodes, %d edges", nodes, edges))
	return strings.Join(parts, "; ")
}

func codeSubgraphText(label string, nodes []sgNode, edges []sgEdge) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Code graph around %s\n", label)
	if len(nodes) > 0 {
		b.WriteString("Nodes:\n")
		for _, n := range nodes {
			fmt.Fprintf(&b, "- %s (%s)\n", n.Label, n.FileType)
		}
	}
	if len(edges) > 0 {
		b.WriteString("Edges:\n")
		for _, e := range edges {
			fmt.Fprintf(&b, "- %s -[%s]-> %s\n", e.Source, e.Relation, e.Target)
		}
	}
	return strings.TrimSpace(b.String())
}

func repomapGist(entry repomap.FileEntry) string {
	names := make([]string, 0, min(len(entry.Symbols), 6))
	for _, sym := range entry.Symbols {
		if sym.Name == "" {
			continue
		}
		names = append(names, sym.Name)
		if len(names) >= 6 {
			break
		}
	}
	if len(names) == 0 {
		return "repo map symbols"
	}
	return "symbols: " + strings.Join(names, ", ")
}

func repomapEntryText(entry repomap.FileEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", entry.Path)
	for _, sym := range entry.Symbols {
		if sym.Signature != "" {
			fmt.Fprintf(&b, "- %s\n", sym.Signature)
		} else {
			fmt.Fprintf(&b, "- %s %s\n", sym.Kind, sym.Name)
		}
	}
	return strings.TrimSpace(b.String())
}

// sourceExt is the code-file extension set used only for the cheap staleness
// walk (a stat-only pass; no parsing). It mirrors the languages gfy detects.
var sourceExt = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".rs": true, ".java": true, ".c": true, ".h": true, ".cpp": true, ".cc": true,
	".rb": true, ".swift": true, ".kt": true, ".cs": true, ".scala": true,
	".php": true, ".lua": true, ".zig": true, ".ex": true, ".jl": true,
	".m": true, ".dart": true, ".vue": true, ".svelte": true,
}

type sgNode struct {
	Label    string `json:"label"`
	FileType string `json:"file_type"`
}

type sgEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
}

// loadOrBuild returns the code graph for root: it loads a fresh cache when one
// exists and no source file is newer, otherwise it rebuilds and best-effort
// caches. forceRebuild skips the cache entirely; quiet suppresses build
// progress (used in --json mode so stderr stays clean).
func loadOrBuild(rc *tool.RunContext, root string, forceRebuild, quiet bool) (*codegraph.GraphContext, error) {
	cp := filepath.Join(root, cacheRel)
	if !forceRebuild && cacheFresh(root, cp) {
		if gc, err := codegraph.Load(cp); err == nil && gc != nil {
			return gc, nil
		}
	}
	var progress codegraph.ProgressFunc
	if !quiet {
		progress = func(msg string) { fmt.Fprintln(rc.Err, msg) }
	}
	gc, err := codegraph.BuildWithProgress(root, progress)
	if err != nil {
		return nil, err
	}
	if err := gc.Save(cp); err != nil && !quiet {
		fmt.Fprintf(rc.Err, "graph: cache write skipped: %v\n", err)
	}
	return gc, nil
}

func cacheFresh(root, cachePath string) bool {
	ci, err := os.Stat(cachePath)
	if err != nil {
		return false
	}
	if err := codegraph.CacheFresh(cachePath); err != nil {
		return false
	}
	return !newestSourceMTime(root).After(ci.ModTime())
}

func newestSourceMTime(root string) time.Time {
	var newest time.Time
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".agents", ".weave", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !sourceExt[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

func nodeLabel(g *gfygraph.Graph, id string) string {
	if s, ok := g.NodeAttrs(id)["label"].(string); ok {
		return s
	}
	return id
}

func edgeStr(attrs map[string]any, key string) string {
	if v, ok := attrs[key].(string); ok {
		return v
	}
	return ""
}

// subgraphView builds the node list from visited (kept) ids and only the edges
// whose BOTH endpoints are kept, so a capped result stays internally consistent.
func subgraphView(g *gfygraph.Graph, visited []string, edges []gfygraph.EdgeData) ([]sgNode, []sgEdge) {
	kept := make(map[string]bool, len(visited))
	for _, id := range visited {
		kept[id] = true
	}
	nodes := make([]sgNode, 0, len(visited))
	for _, nid := range visited {
		a := g.NodeAttrs(nid)
		nodes = append(nodes, sgNode{Label: edgeStr(a, "label"), FileType: edgeStr(a, "file_type")})
	}
	sgEdges := make([]sgEdge, 0, len(edges))
	for _, e := range edges {
		if !kept[e.Source] || !kept[e.Target] {
			continue
		}
		sgEdges = append(sgEdges, sgEdge{
			Source:   nodeLabel(g, e.Source),
			Target:   nodeLabel(g, e.Target),
			Relation: edgeStr(e.Attrs, "relation"),
		})
	}
	return nodes, sgEdges
}
