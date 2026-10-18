// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/coreutils/tool"
	dbcmd "github.com/qiangli/yoke/cmds/db"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/ref"
)

// This file covers the Wave2 lexical-retrieval consumer mount (story
// a566e6387674): `kb memory retrieve` and `kb memory index-sync` run the
// producer's constructors through the same `kb` tree the runtime dispatches,
// with the same exit codes and MCP filtering as capture. Short filtered
// in-process tests only; the manager runs the whole package and live CLI/MCP
// remotely.

func kbMemoryIndexTarget(path string) string {
	return (ref.Locator{Scheme: "db", Target: "bleve:file:" + path}).String()
}

// runKBMemorySub runs `kb memory SUB ...` through the same tree the runtime
// dispatch executes and returns stdout plus the exit code it would report.
func runKBMemorySub(t *testing.T, sub string, args ...string) (string, int) {
	t.Helper()
	cmd := schemaKBCmd()
	var stdout, stderr strings.Builder
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"memory", sub}, args...))
	if err := cmd.Execute(); err != nil {
		return stdout.String(), kbExitCode(err)
	}
	return stdout.String(), 0
}

// retrievalIsolation mirrors the producer's test isolation for the two
// stores retrieval touches: the owner coordination dir and the capture
// episode provenance. The kb single-store isolation comes from
// isolateKBCommandTest.
func retrievalIsolation(t *testing.T) string {
	t.Helper()
	root := isolateKBCommandTest(t)
	t.Chdir(root)
	t.Setenv("BASHY_COORD_DIR", filepath.Join(root, "coord"))
	t.Setenv("BASHY_DB_SESSION_DIR", filepath.Join(root, "sessions"))
	t.Setenv("BASHY_EPISODE", "ep-test")
	t.Setenv("BASHY_PARENT_EPISODE", "ep-parent")
	ringDir(t)
	return root
}

func decodeRetrievalEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid retrieval envelope %q: %v", out, err)
	}
	return env
}

// createBleveIndex creates the rank index the sync reconciles. Retrieval
// never provisions an index, so the operator (here the test) creates it
// explicitly through the same db surface production uses.
func createBleveIndex(t *testing.T, dir, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stdout, stderr, code := dbcmd.MCPCommand().Run(ctx,
		[]string{"--engine", "bleve", "--mode", "persistent", "--path", path, "create"}, "", dir)
	if code != 0 {
		t.Fatalf("create bleve index: exit %d stdout %s stderr %s", code, stdout, stderr)
	}
}

func addMemoryRecord(t *testing.T, target string, args ...string) string {
	t.Helper()
	full := append([]string{"add", target, "--json"}, args...)
	out, code := runKBMemory(t, full...)
	if code != 0 {
		t.Fatalf("add: exit %d %s", code, out)
	}
	var env struct {
		Result struct {
			Record struct {
				ID string `json:"id"`
			} `json:"record"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Result.Record.ID == "" {
		t.Fatalf("add: invalid envelope %q: %v", out, err)
	}
	return env.Result.Record.ID
}

func TestKBMemoryRetrievalSchemaRegistered(t *testing.T) {
	schemas := cobraSchemas("kb")
	for _, path := range []string{"kb memory retrieve", "kb memory index-sync"} {
		schema, ok := schemas[path]
		if !ok {
			t.Fatalf("%s missing from the kb schema tree", path)
		}
		if !slices.ContainsFunc(schema.Flags, func(f tool.ArgFlag) bool { return f.Name == "index" }) {
			t.Fatalf("%s schema lacks --index: %+v", path, schema.Flags)
		}
	}
	retrieve := schemas["kb memory retrieve"]
	for _, name := range []string{"text", "mode", "tags", "limit", "json"} {
		if !slices.ContainsFunc(retrieve.Flags, func(f tool.ArgFlag) bool { return f.Name == name }) {
			t.Fatalf("kb memory retrieve schema lacks --%s: %+v", name, retrieve.Flags)
		}
	}
	if _, ok := schemas["kb recall"]; !ok {
		t.Fatal("mounting retrieval dropped kb recall from the schema tree")
	}
	if _, ok := schemas["kb memory"]; !ok {
		t.Fatal("mounting retrieval dropped kb memory from the schema tree")
	}
}

// TestKBMemoryRetrievalLexicalFlow is the whole consumer path end to end:
// capture init/add through the mounted tree, explicit index creation and
// index-sync, then lexical recall serving a stable ref, a score and the
// actual mode with a fresh population report. Semantic and hybrid fail with
// explicit unsupported and never touch the index.
func TestKBMemoryRetrievalLexicalFlow(t *testing.T) {
	root := retrievalIsolation(t)
	docPath := filepath.Join(root, "mem.db")
	idxPath := filepath.Join(root, "mem.bleve")
	target, index := kbMemoryTarget(docPath), kbMemoryIndexTarget(idxPath)

	if out, code := runKBMemory(t, "init", target, "--json"); code != 0 {
		t.Fatalf("init: exit %d %s", code, out)
	}
	wheatID := addMemoryRecord(t, target, "--body", "wheat harvest ledger for the north field", "--tags", "harvest")
	addMemoryRecord(t, target, "--body", "rye field notes for the south field")
	createBleveIndex(t, root, idxPath)

	// Before the explicit sync the new records are unindexed: recall serves
	// nothing and reports the population stale rather than claiming fresh.
	out, code := runKBMemorySub(t, "retrieve", target, "--index", index, "--text", "wheat", "--json")
	env := decodeRetrievalEnvelope(t, out)
	if code != 0 || env["schema_version"] != "bashy-kb-memory-retrieval-v1" {
		t.Fatalf("pre-sync retrieve: exit %d %s", code, out)
	}
	if fresh, _ := env["result"].(map[string]any)["freshness"].(map[string]any)["stale"].(bool); !fresh {
		t.Fatalf("pre-sync retrieve should report stale, got %s", out)
	}
	if hits, _ := env["result"].(map[string]any)["hits"].([]any); len(hits) != 0 {
		t.Fatalf("pre-sync retrieve should serve no hits, got %s", out)
	}

	out, code = runKBMemorySub(t, "index-sync", target, "--index", index, "--json")
	env = decodeRetrievalEnvelope(t, out)
	if code != 0 {
		t.Fatalf("index-sync: exit %d %s", code, out)
	}
	if added, _ := env["result"].(map[string]any)["added"].(float64); added != 2 {
		t.Fatalf("index-sync should add 2, got %s", out)
	}

	out, code = runKBMemorySub(t, "retrieve", target, "--index", index, "--text", "wheat", "--json")
	env = decodeRetrievalEnvelope(t, out)
	if code != 0 {
		t.Fatalf("retrieve: exit %d %s", code, out)
	}
	res, _ := env["result"].(map[string]any)
	if res["mode"] != "lexical" {
		t.Fatalf("retrieve should report the actual lexical mode, got %s", out)
	}
	hits, _ := res["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("retrieve should serve 1 hit, got %s", out)
	}
	hit, _ := hits[0].(map[string]any)
	if hit["ref"] != wheatID {
		t.Fatalf("hit ref %v should be the stable record ID %s", hit["ref"], wheatID)
	}
	if score, _ := hit["score"].(float64); score <= 0 {
		t.Fatalf("hit should carry a positive score, got %s", out)
	}
	if hit["mode"] != "lexical" {
		t.Fatalf("hit should carry the actual lexical mode, got %s", out)
	}
	if stale, _ := res["freshness"].(map[string]any)["stale"].(bool); stale {
		t.Fatalf("post-sync retrieve should report fresh, got %s", out)
	}

	// Tag scope narrows to the tagged record; a tag nothing carries serves
	// nothing while staying fresh.
	out, code = runKBMemorySub(t, "retrieve", target, "--index", index, "--tags", "harvest", "--json")
	env = decodeRetrievalEnvelope(t, out)
	if hits, _ := env["result"].(map[string]any)["hits"].([]any); code != 0 || len(hits) != 1 {
		t.Fatalf("tag filter should serve 1 hit: exit %d %s", code, out)
	}
	out, code = runKBMemorySub(t, "retrieve", target, "--index", index, "--tags", "no-such-tag", "--json")
	env = decodeRetrievalEnvelope(t, out)
	if hits, _ := env["result"].(map[string]any)["hits"].([]any); code != 0 || len(hits) != 0 {
		t.Fatalf("unmatched tag should serve no hits: exit %d %s", code, out)
	}

	// A future time window excludes every record without going stale.
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	out, code = runKBMemorySub(t, "retrieve", target, "--index", index, "--since", future, "--json")
	env = decodeRetrievalEnvelope(t, out)
	if hits, _ := env["result"].(map[string]any)["hits"].([]any); code != 0 || len(hits) != 0 {
		t.Fatalf("future window should serve no hits: exit %d %s", code, out)
	}
	if stale, _ := env["result"].(map[string]any)["freshness"].(map[string]any)["stale"].(bool); stale {
		t.Fatalf("filtered browse should stay fresh, got %s", out)
	}

	// Semantic and hybrid are explicit unsupported, never a silent lexical
	// substitution.
	for _, mode := range []string{"semantic", "hybrid"} {
		out, code = runKBMemorySub(t, "retrieve", target, "--index", index, "--text", "wheat", "--mode", mode, "--json")
		if code != 2 || !strings.Contains(out, `"unsupported"`) {
			t.Fatalf("%s: exit %d %s, want explicit unsupported", mode, code, out)
		}
	}
}

// TestKBMemoryRetrievalRefusesInheritedScope mirrors the capture rule: a
// memory TARGET is named explicitly, so an inherited kb store selector is
// refused before anything is read or written.
func TestKBMemoryRetrievalRefusesInheritedScope(t *testing.T) {
	for _, sub := range []string{"retrieve", "index-sync"} {
		for _, flag := range []string{"--dir", "--repo", "--user", "--base-dir"} {
			t.Run(sub+"/"+strings.TrimPrefix(flag, "--"), func(t *testing.T) {
				root := retrievalIsolation(t)
				docPath := filepath.Join(root, "mem.db")
				idxPath := filepath.Join(root, "mem.bleve")
				args := []string{kbMemoryTarget(docPath), "--index", kbMemoryIndexTarget(idxPath), "--json", flag}
				if flag == "--dir" || flag == "--base-dir" {
					args = append(args, root)
				}
				out, code := runKBMemorySub(t, sub, args...)
				if code != 2 || !strings.Contains(out, `"invalid_input"`) || !strings.Contains(out, "a memory TARGET is named explicitly") {
					t.Fatalf("%s %s: exit %d %s, want invalid_input refusal", sub, flag, code, out)
				}
				for _, path := range []string{docPath, idxPath} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("%s %s refusal touched %s: %v", sub, flag, path, err)
					}
				}
			})
		}
	}
}

// TestKBMemoryRetrievalNoImplicitStore: reads and the explicit sync against
// absent targets fail without provisioning anything, and kb's own
// single-store hook never runs.
func TestKBMemoryRetrievalNoImplicitStore(t *testing.T) {
	root := retrievalIsolation(t)
	docPath := filepath.Join(root, "mem.db")
	idxPath := filepath.Join(root, "mem.bleve")
	target, index := kbMemoryTarget(docPath), kbMemoryIndexTarget(idxPath)

	if out, code := runKBMemorySub(t, "retrieve", target, "--index", index, "--text", "wheat", "--json"); code == 0 || !strings.Contains(out, "bashy-kb-memory-error-v1") {
		t.Fatalf("retrieve before init: exit %d %s", code, out)
	}
	if out, code := runKBMemorySub(t, "index-sync", target, "--index", index, "--json"); code == 0 || !strings.Contains(out, "bashy-kb-memory-error-v1") {
		t.Fatalf("index-sync before init: exit %d %s", code, out)
	}
	for _, path := range []string{docPath, idxPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("refused read created %s: %v", path, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "host-kb"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("retrieval touched the host kb store: %v %v", entries, err)
	}
}

// TestKBMemoryRetrievalMCPRegistration: the retrieval tools ride the
// kb_memory family selector (and their own names), stay hidden otherwise,
// and report the same exit codes as the CLI dispatch.
func TestKBMemoryRetrievalMCPRegistration(t *testing.T) {
	retrievalIsolation(t)

	registered := func(profile string) []string {
		t.Helper()
		opts, err := mcpOptions("", profile)
		if err != nil {
			t.Fatal(err)
		}
		opts.Policy.Audit = func(yokemcp.Record) {}
		var names []string
		for _, command := range opts.Registered() {
			names = append(names, command.Name)
		}
		return names
	}
	family := []string{"kb_memory", "kb_memory_retrieve", "kb_memory_index_sync"}
	for _, name := range family {
		if names := registered("kb_memory"); !slices.Contains(names, name) {
			t.Fatalf("--tools kb_memory missing %s: %v", name, names)
		}
		if names := registered("default"); !slices.Contains(names, name) {
			t.Fatalf("default profile missing %s: %v", name, names)
		}
		if names := registered("all"); !slices.Contains(names, name) {
			t.Fatalf("--tools all missing %s: %v", name, names)
		}
	}
	if names := registered("kb_memory_retrieve"); !slices.Contains(names, "kb_memory_retrieve") ||
		slices.Contains(names, "kb_memory") || slices.Contains(names, "kb_memory_index_sync") {
		t.Fatalf("--tools kb_memory_retrieve should expose only itself: %v", names)
	}
	if names := registered("kb_memory_index_sync"); !slices.Contains(names, "kb_memory_index_sync") ||
		slices.Contains(names, "kb_memory") || slices.Contains(names, "kb_memory_retrieve") {
		t.Fatalf("--tools kb_memory_index_sync should expose only itself: %v", names)
	}
	for _, name := range family {
		if names := registered("db_lifecycle"); slices.Contains(names, name) {
			t.Fatalf("--tools db_lifecycle must not expose %s: %v", name, names)
		}
	}

	// CLI/MCP exit parity through the front door on the same failures.
	root := retrievalIsolation(t)
	target := kbMemoryTarget(filepath.Join(root, "absent.db"))
	index := kbMemoryIndexTarget(filepath.Join(root, "absent.bleve"))
	opts, err := mcpOptions("", "kb_memory")
	if err != nil {
		t.Fatal(err)
	}
	opts.Policy.Audit = func(yokemcp.Record) {}
	ctx, client, _ := mcpFrontClient(t, opts)
	listed, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range family {
		if !slices.ContainsFunc(listed.Tools, func(item *mcpsdk.Tool) bool { return item.Name == name }) {
			t.Fatalf("%s not served for --tools kb_memory", name)
		}
	}
	_, cli := runKBMemorySub(t, "retrieve", target, "--index", index, "--text", "wheat", "--json")
	out, _ := dbMCPResult(t, mcpSessionCall(t, ctx, client, "kb_memory_retrieve",
		map[string]any{"target": target, "index": index, "text": "wheat", "json": true}).StructuredContent)
	if cli == 0 || out.ExitCode != cli {
		t.Fatalf("retrieve absent: cli exit %d, mcp exit %d (%s)", cli, out.ExitCode, out.Stdout)
	}
	_, cliMode := runKBMemorySub(t, "retrieve", target, "--index", index, "--mode", "semantic", "--json")
	outMode, _ := dbMCPResult(t, mcpSessionCall(t, ctx, client, "kb_memory_retrieve",
		map[string]any{"target": target, "index": index, "mode": "semantic", "json": true}).StructuredContent)
	if cliMode != 2 || outMode.ExitCode != cliMode {
		t.Fatalf("retrieve semantic: cli exit %d, mcp exit %d (%s)", cliMode, outMode.ExitCode, outMode.Stdout)
	}
}
