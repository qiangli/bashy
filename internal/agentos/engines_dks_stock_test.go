// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines

package agentos

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// runStockDksDispatch drives the stock build's REAL `bashy dks` early
// dispatch in-process and returns the exit status plus both streams. The
// client verbs print help and status to stdout and refusals to stderr, so
// both matter to the assertions. Like runLeanDksDispatch in the federation
// test, dispatchExit panics a private frontDoorExit while the front door is
// observing, which keeps the test in-process — no VM, no network.
func runStockDksDispatch(t *testing.T, argv []string) (code int, stdout, stderr string) {
	t.Helper()
	oldArgs := os.Args
	os.Args = argv
	t.Cleanup(func() { os.Args = oldArgs })
	frontDoorObserving.Store(true)
	defer frontDoorObserving.Store(false)

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	outDone, errDone := make(chan string), make(chan string)
	go func() { b, _ := io.ReadAll(outR); outDone <- string(b) }()
	go func() { b, _ := io.ReadAll(errR); errDone <- string(b) }()

	func() {
		defer func() {
			os.Stdout, os.Stderr = oldOut, oldErr
			_ = outW.Close()
			_ = errW.Close()
			v := recover()
			if v == nil {
				t.Error("dispatchEngine returned without exiting")
				return
			}
			c, ok := v.(frontDoorExit)
			if !ok {
				panic(v)
			}
			code = int(c)
		}()
		dispatchEngine("dks")
	}()

	stdout, stderr = <-outDone, <-errDone
	_ = outR.Close()
	_ = errR.Close()
	return code, stdout, stderr
}

// isolateDksKubeEnv hermetically isolates kubeconfig selection: no explicit
// $KUBECONFIG, no $DKS_PROFILE, and a throwaway HOME so the standard
// ~/.kube/config is absent (falls back to the implicit cloudbox path without
// touching the developer's real files).
func isolateDksKubeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"KUBECONFIG", "DKS_PROFILE"} {
		if v, ok := os.LookupEnv(k); ok {
			t.Setenv(k, v)
			if err := os.Unsetenv(k); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestStockDksHelpResolves pins the story contract: on the stock build
// (no bashy_engines tag) `bashy dks --help` — and bare `bashy dks` — resolve
// to the client help instead of the old engine refusal.
func TestStockDksHelpResolves(t *testing.T) {
	for _, argv := range [][]string{
		{"bashy", "dks", "--help"},
		{"bashy", "dks", "-h"},
		{"bashy", "dks"},
	} {
		code, stdout, stderr := runStockDksDispatch(t, argv)
		if code != 0 {
			t.Fatalf("%v: exit = %d, want 0; stderr:\n%s", argv, code, stderr)
		}
		for _, want := range []string{"status", "kubeconfig", "shell", "federation", "provision"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("%v: help missing %q; stdout:\n%s", argv, want, stdout)
			}
		}
		if strings.Contains(stderr, "not available in this build") {
			t.Errorf("%v: help hit the engine refusal; stderr:\n%s", argv, stderr)
		}
	}
}

// TestStockDksStatusHelpResolves pins `bashy dks status --help`: the
// client-side status verb must resolve on the stock build.
func TestStockDksStatusHelpResolves(t *testing.T) {
	code, stdout, stderr := runStockDksDispatch(t, []string{"bashy", "dks", "status", "--help"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "usage:") {
		t.Errorf("status --help printed no usage; stdout:\n%s", stdout)
	}
}

// TestStockDksStatusReports pins that `bashy dks status` itself works on the
// stock build: human output names the kubeconfig plane, and --json decodes to
// the bashy-dks-status-v1 shape with build=stock.
func TestStockDksStatusReports(t *testing.T) {
	isolateDksKubeEnv(t)

	code, stdout, stderr := runStockDksDispatch(t, []string{"bashy", "dks", "status"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "kubeconfig") {
		t.Errorf("status names no kubeconfig plane; stdout:\n%s", stdout)
	}

	code, stdout, stderr = runStockDksDispatch(t, []string{"bashy", "dks", "status", "--json"})
	if code != 0 {
		t.Fatalf("--json: exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	var st struct {
		Build      string `json:"build"`
		Kubeconfig string `json:"kubeconfig"`
	}
	if err := json.Unmarshal([]byte(stdout), &st); err != nil {
		t.Fatalf("--json is not JSON: %v; stdout:\n%s", err, stdout)
	}
	if st.Build != "stock" {
		t.Errorf("--json build = %q, want stock", st.Build)
	}
}

// TestStockDksClientHelpResolves pins that the satellite verbs' help resolves
// locally — no outpost agent needed to discover them.
func TestStockDksClientHelpResolves(t *testing.T) {
	for _, verb := range []string{"kubeconfig", "shell"} {
		code, stdout, stderr := runStockDksDispatch(t, []string{"bashy", "dks", verb, "--help"})
		if code != 0 {
			t.Fatalf("%s --help: exit = %d, want 0; stderr:\n%s", verb, code, stderr)
		}
		if !strings.Contains(stdout, "usage:") {
			t.Errorf("%s --help printed no usage; stdout:\n%s", verb, stdout)
		}
	}
}

// TestStockDksProvisionStillRefuses pins the gated half: the VM-provisioning
// verb keeps refusing on the stock build with the clear host/engine message.
func TestStockDksProvisionStillRefuses(t *testing.T) {
	code, _, stderr := runStockDksDispatch(t, []string{"bashy", "dks", "provision"})
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr:\n%s", code, stderr)
	}
	for _, want := range []string{"not available in this build", "host/engine build"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("provision refusal missing %q; stderr:\n%s", want, stderr)
		}
	}
}

// TestStockDksUnknownRefuses pins the fail-closed door for anything that is
// neither a stock client verb nor yoke's federation/userland surface: exit
// non-zero with a pointer to --help, never a silent success.
func TestStockDksUnknownRefuses(t *testing.T) {
	code, _, stderr := runStockDksDispatch(t, []string{"bashy", "dks", "bogusverb"})
	if code == 0 {
		t.Fatalf("exit = 0, want non-zero for an unknown subcommand")
	}
	if !strings.Contains(stderr, "unknown subcommand") {
		t.Errorf("unknown verb printed no unknown-subcommand message; stderr:\n%s", stderr)
	}
}
