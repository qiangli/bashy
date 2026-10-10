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
		Ref:    coord.Ref{Kind: "ollama", Name: "10.0.0.7:11500"},
		Holder: coord.Self(),
		Intent: "serving on node 2",
	})
	if err != nil {
		t.Fatalf("acquire ollama:10.0.0.7:11500: %v", err)
	}

	// Switch to non-holder.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	// Default instance "bashy" is uncontested, returns 0.
	t.Setenv("OLLAMA_HOST", "")
	if code := guardOllama([]string{"run", "llama3"}); code != 0 {
		t.Fatalf("guardOllama(default) returned %d, want 0", code)
	}

	// The endpoint the engine is routed to names the claim, whatever its spelling.
	for _, host := range []string{
		"10.0.0.7:11500",
		"http://10.0.0.7:11500",
		"http://user:pw@10.0.0.7:11500/k/secret-token/s/sess",
		"HTTP://10.0.0.7:11500/",
	} {
		t.Setenv("OLLAMA_HOST", host)
		var code int
		captured := captureStderr(t, func() {
			code = guardOllama([]string{"run", "llama3"})
		})
		if code != coordExitRefused {
			t.Errorf("OLLAMA_HOST=%q: guardOllama = %d, want %d", host, code, coordExitRefused)
		}
		if strings.Contains(captured, "secret-token") || strings.Contains(captured, "pw@") {
			t.Errorf("OLLAMA_HOST=%q: claim name leaked credentials: %s", host, captured)
		}
	}
}

// A selector the router does not read must not choose the claim: setting it
// would check one instance while the engine still routes to another.
func TestGuardOllama_SelectorsThatDoNotRouteDoNotChooseTheClaim(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")
	t.Setenv("OLLAMA_HOST", "")
	for _, name := range []string{"bashy", "unused"} {
		if _, err := coord.AcquireRef(context.Background(), coord.Request{
			Ref: coord.Ref{Kind: "ollama", Name: name}, Holder: coord.Self(), Intent: "serving",
		}); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	t.Setenv("BASHY_OLLAMA_INSTANCE", "free-name")
	for _, args := range [][]string{{"run", "llama3"}, {"--instance", "free-name", "run", "llama3"}, {"--instance=free-name", "run"}} {
		var code int
		captured := captureStderr(t, func() { code = guardOllama(args) })
		if code != coordExitRefused || !strings.Contains(captured, "ollama:bashy") {
			t.Errorf("guardOllama(%v) = %d (%q), want refusal on the routed managed instance ollama:bashy", args, code, captured)
		}
	}
}

func TestOllamaInstanceName_ManagedEndpointsAreTheDefaultInstance(t *testing.T) {
	for _, host := range []string{"", "127.0.0.1:11435", "http://127.0.0.1:11435/k/tok", "localhost:11435"} {
		t.Setenv("OLLAMA_HOST", host)
		if got := ollamaInstanceName(); got != "bashy" {
			t.Errorf("OLLAMA_HOST=%q -> %q, want bashy", host, got)
		}
	}
	for host, want := range map[string]string{
		"127.0.0.1:11500":       "127.0.0.1:11500",
		"localhost:11500":       "127.0.0.1:11500",
		":11500":                "127.0.0.1:11500",
		"example.com":           "example.com:11434",
		"https://O.example.com": "o.example.com:443",
		"[::1]:9":               "[::1]:9",
	} {
		t.Setenv("OLLAMA_HOST", host)
		if got := ollamaInstanceName(); got != want {
			t.Errorf("OLLAMA_HOST=%q -> %q, want %q", host, got, want)
		}
	}
}

func TestPodmanMachineName_ResolvesGlobalFlagsOnly(t *testing.T) {
	t.Setenv("CONTAINER_CONNECTION", "")
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"run", "-c", "512", "alpine", "true"}, "bashy"},
		{[]string{"run", "--connection", "evil", "alpine"}, "bashy"},
		{[]string{"run", "alpine", "sh", "-c", "echo hi"}, "bashy"},
		{[]string{"run", "--cpu-shares=2", "-c=512", "alpine"}, "bashy"},
		{[]string{"exec", "ctr", "--machine", "x"}, "bashy"},
		{[]string{"-c", "vm1", "run", "-c", "512", "alpine"}, "vm1"},
		{[]string{"--connection=vm2", "ps"}, "vm2"},
		{[]string{"-c=vm3", "ps"}, "vm3"},
		{[]string{"-cvm4", "ps"}, "vm4"},
		{[]string{"--machine", "vm5", "ps"}, "vm5"},
		{[]string{"--remote", "--log-level", "debug", "-c", "vm6", "ps"}, "vm6"},
		{[]string{"--root", "-c", "ps"}, "bashy"},
		{[]string{"--log-level=debug", "ps", "-c", "x"}, "bashy"},
		{[]string{"machine", "start", "vm7"}, "vm7"},
		{[]string{"--log-level", "debug", "machine", "ssh", "vm8"}, "vm8"},
		{[]string{"run", "alpine", "machine", "start", "vm9"}, "bashy"},
		{[]string{"--", "-c", "vm10"}, "bashy"},
	}
	for _, c := range cases {
		if got := podmanMachineName(c.args); got != c.want {
			t.Errorf("podmanMachineName(%v) = %q, want %q", c.args, got, c.want)
		}
	}
	t.Setenv("CONTAINER_CONNECTION", "envvm")
	if got := podmanMachineName([]string{"run", "-c", "512", "alpine"}); got != "envvm" {
		t.Errorf("payload -c must fall through to CONTAINER_CONNECTION, got %q", got)
	}
}

func TestGuardSandbox_PayloadArgumentsCannotRedirectTheClaim(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("CONTAINER_CONNECTION", "")
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")
	if _, err := coord.AcquireRef(context.Background(), coord.Request{
		Ref: coord.Ref{Kind: "sandbox", Name: "bashy"}, Holder: coord.Self(), Intent: "tests",
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	for _, args := range [][]string{
		{"run", "-c", "512", "alpine", "true"},
		{"run", "alpine", "sh", "-c", "true"},
		{"run", "--connection", "unclaimed", "alpine"},
	} {
		var code int
		captureStderr(t, func() { code = guardSandbox(args) })
		if code != coordExitRefused {
			t.Errorf("guardSandbox(%v) = %d, want %d (default sandbox:bashy is held)", args, code, coordExitRefused)
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
