package agentos

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/policy/coord"
	"github.com/qiangli/yoke/pkg/principal"
)

// stubLedgerFault makes every guard see an infrastructure fault from the ledger
// (not a *coord.Conflict) and resets the once-per-process warning so each test
// observes its own first print.
func stubLedgerFault(t *testing.T, fault error) {
	t.Helper()
	setupIsolatedCoord(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	oldGuard, oldEnforce := claimGuardFn, coordEnforceFn
	claimGuardFn = func(context.Context, principal.Ref, ...coord.Use) error { return fault }
	coordEnforceFn = func([]string, string) error { return fault }
	claimCheckWarned.Store(false)
	t.Cleanup(func() {
		claimGuardFn, coordEnforceFn = oldGuard, oldEnforce
		claimCheckWarned.Store(false)
	})
}

func TestClaimGuardMiddleware_LedgerFaultFailsOpenAndWarnsOnce(t *testing.T) {
	fault := errors.New("lock /nowhere/coord: permission denied")
	stubLedgerFault(t, fault)

	calls := 0
	next := func(ctx context.Context, args []string) error {
		calls++
		return nil
	}
	mw := claimGuardMiddleware(next)

	var errs []error
	captured := captureStderr(t, func() {
		for _, argv := range [][]string{{"deploy", "prod"}, {"bashy", "deploy"}, {"make", "test"}} {
			errs = append(errs, mw(context.Background(), argv))
		}
	})

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: ledger fault blocked the command: %v", i, err)
		}
	}
	if calls != 3 {
		t.Fatalf("next called %d times, want 3", calls)
	}
	want := "bashy: claim check unavailable: " + fault.Error() + "; proceeding\n"
	if strings.Count(captured, "claim check unavailable") != 1 || !strings.Contains(captured, want) {
		t.Fatalf("want exactly one warning %q, got stderr:\n%s", want, captured)
	}
}

func TestClaimGuardMiddleware_StubbedConflictStillExit9(t *testing.T) {
	stubLedgerFault(t, &coord.Conflict{Claim: &coord.Claim{
		Kind: "command", Resource: "deploy", Intent: "rollout",
		Holder: principal.Ref{Name: "holder-agent", Host: "lab"},
	}})

	nextCalled := false
	mw := claimGuardMiddleware(func(context.Context, []string) error {
		nextCalled = true
		return nil
	})

	var runErr error
	captured := captureStderr(t, func() {
		runErr = mw(context.Background(), []string{"deploy", "prod"})
	})
	if nextCalled {
		t.Fatal("next handler ran despite a conflict")
	}
	if status, ok := exitStatusOf(runErr); !ok || status != coordExitRefused {
		t.Fatalf("exit status = %d (ok=%v), want %d; err = %v", status, ok, coordExitRefused, runErr)
	}
	if strings.Contains(captured, "claim check unavailable") {
		t.Fatalf("a conflict must not be reported as a ledger fault:\n%s", captured)
	}
	if !strings.Contains(captured, "holder-agent") {
		t.Fatalf("stderr does not name holder:\n%s", captured)
	}
}

func TestEngineGuards_LedgerFaultFailsOpenAndWarnsOnce(t *testing.T) {
	stubLedgerFault(t, errors.New("read coord dir: input/output error"))

	var codes []int
	captured := captureStderr(t, func() {
		codes = append(codes,
			guardSandbox([]string{"ps"}),
			guardOllama([]string{"list"}),
			guardEngine("podman", []string{"ps"}),
			guardEngine("ollama", []string{"list"}),
		)
	})
	for i, code := range codes {
		if code != 0 {
			t.Errorf("guard %d returned %d on a ledger fault, want 0", i, code)
		}
	}
	if n := strings.Count(captured, "claim check unavailable"); n != 1 {
		t.Fatalf("warned %d times, want once:\n%s", n, captured)
	}
}

func TestCoordGuardGit_LedgerFaultFailsOpen(t *testing.T) {
	stubLedgerFault(t, errors.New("flock: resource temporarily unavailable"))

	var code int
	captured := captureStderr(t, func() {
		code = coordGuard([]string{"commit", "-m", "x"})
	})
	if code != 0 {
		t.Fatalf("coordGuard returned %d on a ledger fault, want 0", code)
	}
	if n := strings.Count(captured, "claim check unavailable"); n != 1 {
		t.Fatalf("warned %d times, want once:\n%s", n, captured)
	}

	// A conflict from the same seam still refuses.
	coordEnforceFn = func([]string, string) error {
		return &coord.Conflict{Claim: &coord.Claim{Project: "proj", Holder: principal.Ref{Name: "holder-agent"}}}
	}
	captured = captureStderr(t, func() {
		code = coordGuard([]string{"commit", "-m", "x"})
	})
	if code != coordExitRefused {
		t.Fatalf("coordGuard returned %d on a conflict, want %d", code, coordExitRefused)
	}
	if !strings.Contains(captured, "refusing `git commit") {
		t.Fatalf("stderr does not announce the refusal:\n%s", captured)
	}
}
