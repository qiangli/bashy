// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"path/filepath"
	"slices"
	"testing"

	yokemcp "github.com/qiangli/yoke/mcp"
)

// Consumer mount of `kb memory maintain` and kb_memory_maintain (story
// b9d919218f1a). Short in-process tests only; the manager runs whole-package
// and live CLI/MCP remotely.

func TestKBMemoryMaintainSchemaRegistered(t *testing.T) {
	schemas := cobraSchemas("kb")
	if _, ok := schemas["kb memory maintain"]; !ok {
		t.Fatal("kb memory maintain missing from the kb schema tree")
	}
	for _, path := range []string{"kb memory", "kb memory retrieve", "kb memory index-sync", "kb recall"} {
		if _, ok := schemas[path]; !ok {
			t.Fatalf("mounting maintain dropped %s from the schema tree", path)
		}
	}
}

func TestKBMemoryMaintainExitBehavior(t *testing.T) {
	root := retrievalIsolation(t)
	target := kbMemoryTarget(filepath.Join(root, "absent.db"))
	if _, code := runKBMemorySub(t, "maintain", "candidates", target); code == 0 {
		t.Fatal("maintain on an absent store must fail")
	}
	if _, code := runKBMemorySub(t, "maintain", "bogus", target); code == 0 {
		t.Fatal("unknown maintain verb must fail")
	}
}

func TestKBMemoryMaintainMCPRegistration(t *testing.T) {
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
	for _, profile := range []string{"kb_memory", "default", "all"} {
		if names := registered(profile); !slices.Contains(names, "kb_memory_maintain") {
			t.Fatalf("%s missing kb_memory_maintain: %v", profile, names)
		}
	}
	if names := registered("kb_memory_maintain"); !slices.Contains(names, "kb_memory_maintain") ||
		slices.Contains(names, "kb_memory") || slices.Contains(names, "kb_memory_retrieve") || slices.Contains(names, "kb_memory_index_sync") {
		t.Fatalf("--tools kb_memory_maintain should expose only itself: %v", names)
	}
	if names := registered("db_lifecycle"); slices.Contains(names, "kb_memory_maintain") {
		t.Fatalf("--tools db_lifecycle must not expose kb_memory_maintain: %v", names)
	}
}
