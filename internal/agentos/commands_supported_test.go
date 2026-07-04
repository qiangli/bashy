// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"slices"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/tool"
)

// TestAllListedCommandsAreSupported pins the invariant behind the goal: every
// command `bashy commands` prints must be backed by a real handler —
//   - a shell builtin (in the interpreter's builtin set),
//   - a registered coreutils tool (tool.Lookup != nil), or
//   - a front-door verb with a synopsis (a verb with no synopsis is almost always
//     one with no dispatch handler).
//
// It catches the class of bug where a command is advertised but then errors
// "No such command" / "not in this build" / "No such file" at runtime (docker,
// podman-lean). Pure + fast + cross-platform (runs identically on macOS/Windows).
func TestAllListedCommandsAreSupported(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1") // agent mode so provisioners (go/cmake/clang) are catalogued too
	builtins, core, verbs := commandsCatalog()
	if len(builtins) == 0 || len(core) == 0 || len(verbs) == 0 {
		t.Fatalf("catalog unexpectedly empty: %d builtins, %d core, %d verbs", len(builtins), len(core), len(verbs))
	}

	realBuiltins := interp.BuiltinNames()
	for _, b := range builtins {
		if !slices.Contains(realBuiltins, b) {
			t.Errorf("builtin %q is listed by `bashy commands` but is not a real interpreter builtin", b)
		}
	}

	for _, c := range core {
		if tool.Lookup(c) == nil {
			t.Errorf("coreutils %q is listed by `bashy commands` but is not registered in the tool registry", c)
		}
	}

	for _, v := range append(append([]string{}, verbs...), hiddenVerbsCatalog()...) {
		if strings.TrimSpace(verbSynopsis[v]) == "" {
			t.Errorf("front-door verb %q is listed by `bashy commands` but has no synopsis (usually means no dispatch handler)", v)
		}
	}
}

// TestDockerAliasIsHandled guards the fix: docker is listed by `bashy commands`
// (an alias for the podman engine), so `bashy docker` must dispatch — it used to
// fall through to "docker: No such file or directory".
func TestDockerAliasIsHandled(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1")
	_, _, verbs := commandsCatalog()
	if !slices.Contains(verbs, "docker") {
		t.Fatal("docker should be listed as a verb")
	}
	if got := engineAlias("docker"); got != "podman" {
		t.Errorf("engineAlias(docker) = %q, want podman", got)
	}
	// Non-aliases pass through unchanged.
	for _, n := range []string{"podman", "ollama", "gh"} {
		if got := engineAlias(n); got != n {
			t.Errorf("engineAlias(%q) = %q, want unchanged", n, got)
		}
	}
}
