// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/kbmemory"
)

// This file covers the bounded-memory-context consumer mount (story
// c537aeb13da6, Yoke 6a236c9): `kb context` serves the producer's explicit
// memory-collection path through the one mounted command — schema flags,
// missing-index refusal, structured failure exit, and MCP schema reuse of
// that same command. Short filtered in-process tests only; the manager runs
// the whole package and live CLI/MCP remotely.

// runKBContext runs `kb context ARGS` through the same tree the runtime
// dispatch executes and returns stdout plus the RunE error.
func runKBContext(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := schemaKBCmd()
	var stdout, stderr strings.Builder
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"context"}, args...))
	return stdout.String(), cmd.Execute()
}

func kbContextFlagSet(t *testing.T) map[string]bool {
	t.Helper()
	schemas := cobraSchemas("kb")
	schema, ok := schemas["kb context"]
	if !ok {
		t.Fatal("kb context missing from the kb schema tree")
	}
	names := make(map[string]bool, len(schema.Flags))
	for _, f := range schema.Flags {
		names[f.Name] = true
	}
	return names
}

// TestKBContextMemoryFlagsInSchema: the explicit collection path is flags
// on the mounted command, visible to both CLI parsing and the MCP verb
// schema derived from this same tree.
func TestKBContextMemoryFlagsInSchema(t *testing.T) {
	names := kbContextFlagSet(t)
	for _, name := range []string{
		"memory-target", "memory-index", "memory-session", "memory-dir",
		"memory-mode", "memory-tags", "memory-repos", "memory-os",
		"memory-since", "memory-until", "memory-wait", "memory-timeout",
	} {
		if !names[name] {
			t.Fatalf("kb context schema lacks --%s", name)
		}
	}
	// Mounting the memory path dropped no sibling surface.
	schemas := cobraSchemas("kb")
	for _, path := range []string{"kb recall", "kb ask", "kb memory"} {
		if _, ok := schemas[path]; !ok {
			t.Fatalf("mounting memory context dropped %s from the schema tree", path)
		}
	}
}

// TestKBContextMemoryNeedsNoSecondTool: memory rides flags on the one
// mounted `kb context` command — the same cobra tree MCP discovery derives
// the kb verb schema from — so no second command or MCP tool exists.
func TestKBContextMemoryNeedsNoSecondTool(t *testing.T) {
	schemas := cobraSchemas("kb")
	if _, ok := schemas["kb context"]; !ok {
		t.Fatal("kb context missing from the kb schema tree")
	}
	for path := range schemas {
		if path != "kb context" &&
			(strings.HasPrefix(path, "kb context ") || strings.HasPrefix(path, "kb memory context")) {
			t.Fatalf("second context command %q: memory rides flags on kb context", path)
		}
	}
}

// TestKBContextMemoryTargetRequiresIndex: naming a collection without its
// rank index is a caller error — indexes are never provisioned — so the
// read refuses before emitting any envelope.
func TestKBContextMemoryTargetRequiresIndex(t *testing.T) {
	root := isolateKBCommandTest(t)
	t.Chdir(root)
	out, err := runKBContext(t, "--for", "wheat ledger", "--memory-target", "bogus", "--json")
	if err == nil || !strings.Contains(err.Error(), "--memory-index is required") {
		t.Fatalf("missing index error = %v, want --memory-index refusal", err)
	}
	if strings.Contains(out, `"context_version"`) {
		t.Fatalf("refused read emitted a context envelope: %s", out)
	}
	// A plain caller error keeps kb's historical exit 1 through the same
	// boundary the runtime dispatch uses.
	if code := kbExitCode(err); code != 1 {
		t.Fatalf("missing index exit = %d, want 1", code)
	}
}

// TestKBContextExplicitTargetFailureCodedExit: with both locators named,
// a retrieval failure is a structured caller error — never a successful
// legacy-only packet. The CLI error carries the producer's code and the
// consumer boundary preserves it; the frozen envelope stays frozen (no
// top-level error field) with legacy blocks withheld.
func TestKBContextExplicitTargetFailureCodedExit(t *testing.T) {
	root := isolateKBCommandTest(t)
	t.Chdir(root)
	out, err := runKBContext(t, "--for", "wheat ledger",
		"--memory-target", "bogus", "--memory-index", "also-bogus", "--json")
	if err == nil {
		t.Fatalf("explicit-collection retrieval failure must not succeed: %s", out)
	}
	var ee *kbmemory.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("explicit failure error = %T (%v), want *kbmemory.ExitError", err, err)
	}
	if ee.Code == 0 {
		t.Fatalf("explicit failure lost its structured code: %+v", ee)
	}
	if got := kbExitCode(err); got != ee.Code {
		t.Fatalf("consumer boundary exit = %d, want structured %d", got, ee.Code)
	}
	var env struct {
		ContextVersion int              `json:"context_version"`
		Blocks         []map[string]any `json:"blocks"`
	}
	if uerr := json.Unmarshal([]byte(out), &env); uerr != nil {
		t.Fatalf("failed packet is not a context envelope: %v (%s)", uerr, out)
	}
	if env.ContextVersion != 1 {
		t.Fatalf("failed packet context_version = %d, want frozen 1", env.ContextVersion)
	}
	if len(env.Blocks) != 0 {
		t.Fatalf("explicit failure served %d legacy blocks, want none withheld", len(env.Blocks))
	}
	if strings.Contains(out, `"memory_error"`) {
		t.Fatalf("failed packet carries a top-level memory_error field: %s", out)
	}
}
