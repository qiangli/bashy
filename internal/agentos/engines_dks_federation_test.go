// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines

package agentos

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/yoke/external/dks"
)

// runLeanDksDispatch drives the lean build's REAL `bashy dks` early dispatch
// in-process with a stubbed federation outpost, and returns the status it
// would exit with. dispatchExit panics a private frontDoorExit while the front
// door is observing, which is what makes the door testable without re-execing
// the binary — and therefore without outpost, a VM, or a network.
func runLeanDksDispatch(t *testing.T, argv []string) (code int, stderr string) {
	t.Helper()
	oldArgs := os.Args
	os.Args = argv
	t.Cleanup(func() { os.Args = oldArgs })
	frontDoorObserving.Store(true)
	defer frontDoorObserving.Store(false)
	stderr = captureStderr(t, func() {
		defer func() {
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
	})
	return code, stderr
}

// TestLeanDksFederationReachesHandler pins the 2026-10-20 correction:
// `bashy dks federation` must be dispatched from the non-engine federation
// package in BOTH builds, so on the lean build (no bashy_engines tag) it
// reaches the federation handler — exec'ing `outpost cluster federation
// status` through the seam — instead of hitting the engine refusal.
func TestLeanDksFederationReachesHandler(t *testing.T) {
	var calls [][]string
	old := dks.ExecOutpost
	dks.ExecOutpost = func(_ context.Context, args ...string) error {
		calls = append(calls, args)
		return nil
	}
	t.Cleanup(func() { dks.ExecOutpost = old })

	code, stderr := runLeanDksDispatch(t, []string{"bashy", "dks", "federation", "status"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
	}
	if strings.Contains(stderr, "not available in this build") {
		t.Errorf("federation hit the lean engine refusal; stderr:\n%s", stderr)
	}
	if len(calls) != 1 || !slices.Equal(calls[0], []string{"cluster", "federation", "status"}) {
		t.Fatalf("outpost calls = %v, want one call [cluster federation status]", calls)
	}
}

// TestLeanDksProvisionStillRefuses pins the other half: the VM-provisioning
// verbs keep their engine-only refusal on the lean build.
func TestLeanDksProvisionStillRefuses(t *testing.T) {
	code, stderr := runLeanDksDispatch(t, []string{"bashy", "dks", "provision"})
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "not available in this build") {
		t.Errorf("provision lost its lean refusal; stderr:\n%s", stderr)
	}
}
