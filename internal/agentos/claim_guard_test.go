package agentos

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/fleet/fleettest"
	"github.com/qiangli/yoke/pkg/policy/coord"
	"github.com/qiangli/yoke/pkg/principal"
)

func TestCoordKindsModelAndCommandExist(t *testing.T) {
	if _, ok := coord.LookupKind("model"); !ok {
		t.Fatal("expected kind 'model' to be registered via fleetkinds")
	}
	if _, ok := coord.LookupKind("command"); !ok {
		t.Fatal("expected kind 'command' to be registered")
	}
}

func setupIsolatedCoord(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BASHY_COORD_DIR", dir)
	t.Setenv("CLAUDE_CODE", "1")
	return dir
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	outCh := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		_ = r.Close()
		outCh <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-outCh
}

func TestClaimGuardMiddleware_HolderPasses(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")

	ctx := context.Background()
	_, err := coord.AcquireRef(ctx, coord.Request{
		Ref:    coord.Ref{Kind: "command", Name: "deploy"},
		Holder: coord.Self(),
		Intent: "production rollout",
	})
	if err != nil {
		t.Fatalf("acquire ref: %v", err)
	}

	nextCalled := false
	next := func(ctx context.Context, args []string) error {
		nextCalled = true
		return nil
	}

	mw := claimGuardMiddleware(next)

	for _, argv := range [][]string{
		{"deploy", "prod"},
		{"bashy", "deploy", "prod"},
		{"command", "bashy", "deploy", "prod"},
	} {
		nextCalled = false
		if err := mw(ctx, argv); err != nil {
			t.Fatalf("argv %v failed for holder: %v", argv, err)
		}
		if !nextCalled {
			t.Fatalf("argv %v did not call next for holder", argv)
		}
	}
}

func TestClaimGuardMiddleware_NonHolderExit9WithContacts(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")

	ctx := context.Background()
	_, err := coord.AcquireRef(ctx, coord.Request{
		Ref:    coord.Ref{Kind: "command", Name: "deploy"},
		Holder: coord.Self(),
		Intent: "production rollout",
	})
	if err != nil {
		t.Fatalf("acquire ref: %v", err)
	}

	// Switch to a non-holder agent.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	nextCalled := false
	next := func(ctx context.Context, args []string) error {
		nextCalled = true
		return nil
	}

	mw := claimGuardMiddleware(next)

	var runErr error
	captured := captureStderr(t, func() {
		runErr = mw(ctx, []string{"deploy", "prod"})
	})

	if nextCalled {
		t.Fatal("next handler was called on conflict, want blocked")
	}

	status, ok := exitStatusOf(runErr)
	if !ok || status != 9 {
		t.Fatalf("exit status = %d (ok=%v), want 9 (coordExitRefused); err = %v", status, ok, runErr)
	}

	if !strings.Contains(captured, "holder-agent") {
		t.Errorf("stderr does not name holder: %s", captured)
	}
	if !strings.Contains(captured, "bashy ping holder-agent") {
		t.Errorf("stderr does not contain contacts: %s", captured)
	}
}

func TestClaimGuardMiddleware_ExemptCommands(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")

	ctx := context.Background()
	// Claim the commands if someone tries to claim them.
	for _, name := range []string{"claim", "inbox", "ping", "mb", "meet", "help"} {
		_, _ = coord.AcquireRef(ctx, coord.Request{
			Ref:    coord.Ref{Kind: "command", Name: name},
			Holder: principal.Ref{Name: "holder-agent"},
		})
	}

	// Switch to non-holder.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")

	nextCalled := false
	next := func(ctx context.Context, args []string) error {
		nextCalled = true
		return nil
	}
	mw := claimGuardMiddleware(next)

	exemptCases := [][]string{
		{"bashy", "claim", "list"},
		{"claim", "list"},
		{"bashy", "inbox"},
		{"inbox"},
		{"bashy", "ping", "holder-agent"},
		{"ping", "holder-agent"},
		{"bashy", "mb"},
		{"mb"},
		{"bashy", "meet"},
		{"meet"},
		{"bashy", "help"},
		{"help"},
		{"bashy"},
	}

	for _, argv := range exemptCases {
		nextCalled = false
		if err := mw(ctx, argv); err != nil {
			t.Errorf("exempt command %v was blocked: %v", argv, err)
		}
		if !nextCalled {
			t.Errorf("exempt command %v did not call next", argv)
		}
	}
}

func TestClaimGuardMiddleware_GateDisables(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")

	ctx := context.Background()
	_, err := coord.AcquireRef(ctx, coord.Request{
		Ref:    coord.Ref{Kind: "command", Name: "deploy"},
		Holder: principal.Ref{Name: "holder-agent"},
	})
	if err != nil {
		t.Fatalf("acquire ref: %v", err)
	}

	// Switch to non-holder.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")

	nextCalled := false
	next := func(ctx context.Context, args []string) error {
		nextCalled = true
		return nil
	}
	mw := claimGuardMiddleware(next)

	// BASHY_CLAIM=0 disables the guard.
	t.Setenv("BASHY_CLAIM", "0")
	nextCalled = false
	if err := mw(ctx, []string{"deploy"}); err != nil || !nextCalled {
		t.Fatalf("BASHY_CLAIM=0 should disable guard: err=%v, nextCalled=%v", err, nextCalled)
	}

	// BASHY_CLAIM=off disables the guard.
	t.Setenv("BASHY_CLAIM", "off")
	nextCalled = false
	if err := mw(ctx, []string{"deploy"}); err != nil || !nextCalled {
		t.Fatalf("BASHY_CLAIM=off should disable guard: err=%v, nextCalled=%v", err, nextCalled)
	}
}

func TestClaimGuardMiddleware_RegisteredCommand(t *testing.T) {
	setupIsolatedCoord(t)
	fleettest.Ring(t)
	fleetDir := t.TempDir()
	t.Setenv("BASHY_FLEET_DIR", fleetDir)
	t.Setenv("BASHY_COMMANDS_DIR", filepath.Join(fleetDir, "commands"))

	cat := fleet.New()
	err := cat.SaveCommand(fleet.Command{
		Name:    "build-engine",
		Aliases: []string{"be"},
		Script:  "echo building",
		Effects: []string{"exec"},
	})
	if err != nil {
		t.Fatalf("save command: %v", err)
	}
	resetRegisteredIndex()

	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")
	ctx := context.Background()
	_, err = coord.AcquireRef(ctx, coord.Request{
		Ref:    coord.Ref{Kind: "command", Name: "build-engine"},
		Holder: coord.Self(),
		Intent: "compilation in progress",
	})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	// Switch to other agent.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	nextCalled := false
	next := func(ctx context.Context, args []string) error {
		nextCalled = true
		return nil
	}
	mw := claimGuardMiddleware(next)

	var runErr error
	captureStderr(t, func() {
		runErr = mw(ctx, []string{"be"})
	})

	if nextCalled {
		t.Fatal("registered command alias should be guarded and not call next")
	}
	status, ok := exitStatusOf(runErr)
	if !ok || status != 9 {
		t.Fatalf("exit status = %d, want 9; err = %v", status, runErr)
	}
}
