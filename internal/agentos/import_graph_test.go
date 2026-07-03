// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBinaryImportGraphsIsolateGfy pins the load-bearing build-hygiene invariant:
// the code-knowledge-graph engine (gfy, pulled by cmds/graph → pkg/codegraph)
// lands in the AgentOS `bashy` binary but NEVER in the lean `bash` drop-in, and
// `bash` never links coreutils at all. If someone moves cmds/graph into cmds/all,
// or imports internal/agentos from cmd/bash, this test fails. Mirrors the spirit
// of coreutils' TestFileTransportIsPureGo (an import-graph guard, not a runtime
// check).
func TestBinaryImportGraphsIsolateGfy(t *testing.T) {
	deps := func(pkg string) string {
		out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
		if err != nil {
			t.Skipf("go list unavailable (%v): %s", err, out)
		}
		return string(out)
	}

	bash := deps("github.com/qiangli/bashy/cmd/bash")
	if strings.Contains(bash, "qiangli/gfy") {
		t.Error("cmd/bash must not import gfy — the lean drop-in stays engine-free")
	}
	if strings.Contains(bash, "qiangli/coreutils") {
		t.Error("cmd/bash must not import coreutils — its import graph is disjoint from AgentOS")
	}

	bashy := deps("github.com/qiangli/bashy/cmd/bashy")
	if !strings.Contains(bashy, "qiangli/gfy") {
		t.Error("cmd/bashy should import gfy — the code-graph feature must be wired in")
	}
	if !strings.Contains(bashy, "qiangli/coreutils/cmds/graph") {
		t.Error("cmd/bashy should import cmds/graph — the graph verbs must be registered")
	}
}
