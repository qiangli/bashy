package agentos

import (
	"context"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/policy/coord"
)

func TestCoordKindsSandboxAndOllamaExist(t *testing.T) {
	if _, ok := coord.LookupKind("sandbox"); !ok {
		t.Fatal("expected kind 'sandbox' to be registered")
	}
	if _, ok := coord.LookupKind("ollama"); !ok {
		t.Fatal("expected kind 'ollama' to be registered")
	}
}

func TestGuardSandbox_DefaultMachine(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")

	ctx := context.Background()
	_, err := coord.AcquireRef(ctx, coord.Request{
		Ref:    coord.Ref{Kind: "sandbox", Name: "bashy"},
		Holder: coord.Self(),
		Intent: "running isolated tests",
	})
	if err != nil {
		t.Fatalf("acquire sandbox:bashy: %v", err)
	}

	// Holder passes.
	if code := guardSandbox([]string{"ps"}); code != 0 {
		t.Fatalf("holder guardSandbox returned %d, want 0", code)
	}

	// Non-holder refused with 9.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	var code int
	captured := captureStderr(t, func() {
		code = guardSandbox([]string{"ps"})
	})
	if code != coordExitRefused {
		t.Fatalf("non-holder guardSandbox returned %d, want %d", code, coordExitRefused)
	}
	if !strings.Contains(captured, "holder-agent") || !strings.Contains(captured, "sandbox:bashy") {
		t.Fatalf("stderr does not name holder or resource: %s", captured)
	}

	// Disabled gate passes.
	t.Setenv("BASHY_CLAIM", "0")
	if code := guardSandbox([]string{"ps"}); code != 0 {
		t.Fatalf("BASHY_CLAIM=0 should allow, got %d", code)
	}
}

func TestGuardSandbox_NamedMachine(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")

	ctx := context.Background()
	_, err := coord.AcquireRef(ctx, coord.Request{
		Ref:    coord.Ref{Kind: "sandbox", Name: "custom-vm"},
		Holder: coord.Self(),
		Intent: "vm run",
	})
	if err != nil {
		t.Fatalf("acquire sandbox:custom-vm: %v", err)
	}

	// Switch to non-holder.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	// Default machine "bashy" is uncontested, returns 0.
	if code := guardSandbox([]string{"ps"}); code != 0 {
		t.Fatalf("guardSandbox(default) returned %d, want 0", code)
	}

	// Different flag shapes for custom-vm:
	for _, args := range [][]string{
		{"--connection", "custom-vm", "ps"},
		{"--connection=custom-vm", "ps"},
		{"-c", "custom-vm", "ps"},
		{"-c=custom-vm", "ps"},
		{"--machine", "custom-vm", "ps"},
		{"--machine=custom-vm", "ps"},
		{"machine", "start", "custom-vm"},
	} {
		var code int
		captureStderr(t, func() {
			code = guardSandbox(args)
		})
		if code != coordExitRefused {
			t.Errorf("guardSandbox(%v) = %d, want %d", args, code, coordExitRefused)
		}
	}
}

func TestGuardOllama_DefaultInstance(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")

	ctx := context.Background()
	_, err := coord.AcquireRef(ctx, coord.Request{
		Ref:    coord.Ref{Kind: "ollama", Name: "bashy"},
		Holder: coord.Self(),
		Intent: "serving models",
	})
	if err != nil {
		t.Fatalf("acquire ollama:bashy: %v", err)
	}

	// Holder passes.
	if code := guardOllama([]string{"run", "glm-5.2"}); code != 0 {
		t.Fatalf("holder guardOllama returned %d, want 0", code)
	}

	// Non-holder refused with 9.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	var code int
	captured := captureStderr(t, func() {
		code = guardOllama([]string{"run", "glm-5.2"})
	})
	if code != coordExitRefused {
		t.Fatalf("non-holder guardOllama returned %d, want %d", code, coordExitRefused)
	}
	if !strings.Contains(captured, "holder-agent") || !strings.Contains(captured, "ollama:bashy") {
		t.Fatalf("stderr does not name holder or resource: %s", captured)
	}

	// BASHY_CLAIM=off disables.
	t.Setenv("BASHY_CLAIM", "off")
	if code := guardOllama([]string{"run", "glm-5.2"}); code != 0 {
		t.Fatalf("BASHY_CLAIM=off should allow, got %d", code)
	}
}

func TestGuardOllama_NamedInstance(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")

	ctx := context.Background()
	_, err := coord.AcquireRef(ctx, coord.Request{
		Ref:    coord.Ref{Kind: "ollama", Name: "node-2"},
		Holder: coord.Self(),
		Intent: "serving on node 2",
	})
	if err != nil {
		t.Fatalf("acquire ollama:node-2: %v", err)
	}

	// Switch to non-holder.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	// Default instance "bashy" is uncontested, returns 0.
	if code := guardOllama([]string{"run", "llama3"}); code != 0 {
		t.Fatalf("guardOllama(default) returned %d, want 0", code)
	}

	for _, args := range [][]string{
		{"--instance", "node-2", "run", "llama3"},
		{"--instance=node-2", "run", "llama3"},
	} {
		var code int
		captureStderr(t, func() {
			code = guardOllama(args)
		})
		if code != coordExitRefused {
			t.Errorf("guardOllama(%v) = %d, want %d", args, code, coordExitRefused)
		}
	}
}

func TestGuardEngine_Aliases(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")

	ctx := context.Background()
	_, _ = coord.AcquireRef(ctx, coord.Request{
		Ref:    coord.Ref{Kind: "sandbox", Name: "bashy"},
		Holder: coord.Self(),
	})
	_, _ = coord.AcquireRef(ctx, coord.Request{
		Ref:    coord.Ref{Kind: "ollama", Name: "bashy"},
		Holder: coord.Self(),
	})

	// Switch to non-holder.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	for _, alias := range []string{"podman", "oci", "sandbox", "docker"} {
		var code int
		captureStderr(t, func() {
			code = guardEngine(alias, []string{"ps"})
		})
		if code != coordExitRefused {
			t.Errorf("guardEngine(%s) = %d, want %d", alias, code, coordExitRefused)
		}
	}

	var code int
	captureStderr(t, func() {
		code = guardEngine("ollama", []string{"run", "model"})
	})
	if code != coordExitRefused {
		t.Errorf("guardEngine(ollama) = %d, want %d", code, coordExitRefused)
	}
}
