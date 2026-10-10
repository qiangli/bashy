package agentos

import (
	"context"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/policy/coord"
	"github.com/qiangli/yoke/pkg/principal"
)

// countGuards stands in for the ledger and records every Guard call's uses.
func countGuards(t *testing.T) *[][]coord.Use {
	t.Helper()
	var calls [][]coord.Use
	old := claimGuardFn
	claimGuardFn = func(_ context.Context, _ principal.Ref, uses ...coord.Use) error {
		calls = append(calls, uses)
		return nil
	}
	t.Cleanup(func() { claimGuardFn = old })
	return &calls
}

func countRegisteredLookups(t *testing.T) *int {
	t.Helper()
	n := 0
	old := claimRegisteredLookup
	claimRegisteredLookup = func(name string) (fleet.Command, bool) {
		n++
		return old(name)
	}
	t.Cleanup(func() { claimRegisteredLookup = old })
	return &n
}

func TestClaimGuardMiddleware_OneGuardCallPerInvocation(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("CONTAINER_CONNECTION", "")
	calls := countGuards(t)
	mw := claimGuardMiddleware(func(context.Context, []string) error { return nil })

	for _, tc := range []struct {
		argv []string
		uses int
	}{
		{[]string{"ls", "-l"}, 1},
		{[]string{"git", "status"}, 1},
		{[]string{"deploy", "prod"}, 1},
		{[]string{"bashy", "podman", "ps"}, 2},
		{[]string{"podman", "run", "-c", "512", "alpine", "true"}, 2},
		{[]string{"command", "bashy", "docker", "ps"}, 2},
		{[]string{"sandbox", "ps"}, 2},
		{[]string{"bashy", "ollama", "list"}, 2},
	} {
		*calls = nil
		if err := mw(context.Background(), tc.argv); err != nil {
			t.Fatalf("%v: %v", tc.argv, err)
		}
		if len(*calls) != 1 || len((*calls)[0]) != tc.uses {
			t.Errorf("%v: guard calls = %v, want exactly one call carrying %d use(s)", tc.argv, *calls, tc.uses)
		}
	}

	*calls = nil
	_ = mw(context.Background(), []string{"podman", "run", "-c", "512", "alpine"})
	got := (*calls)[0]
	if got[0] != (coord.Use{Kind: "command", Name: "podman"}) || got[1] != (coord.Use{Kind: "sandbox", Name: "bashy"}) {
		t.Errorf("podman uses = %v, want command:podman + sandbox:bashy", got)
	}
}

func TestFrontDoorEngineReusesTheMiddlewareGuardCall(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("CONTAINER_CONNECTION", "")
	t.Setenv("OLLAMA_HOST", "")
	for _, argv := range [][]string{{"podman", "ps"}, {"docker", "ps"}, {"ollama", "list"}} {
		calls := countGuards(t)
		frontDoorObserving.Store(true)
		engineClaimCovered.Store(false)
		t.Cleanup(func() { frontDoorObserving.Store(false); engineClaimCovered.Store(false) })

		var doorCode int
		mw := claimGuardMiddleware(func(context.Context, []string) error {
			doorCode = guardEngine(argv[0], argv[1:]) // what dispatchEngine does first
			return nil
		})
		if err := mw(context.Background(), argv); err != nil || doorCode != 0 {
			t.Fatalf("%v: err=%v door=%d", argv, err, doorCode)
		}
		if len(*calls) != 1 {
			t.Errorf("%v: front-door invocation made %d Guard calls %v, want 1", argv, len(*calls), *calls)
		}
	}
}

func TestEngineGuardStillRunsWhenNoMiddlewareCoveredIt(t *testing.T) {
	setupIsolatedCoord(t)
	t.Setenv("CONTAINER_CONNECTION", "")
	engineClaimCovered.Store(false)
	frontDoorObserving.Store(false)
	calls := countGuards(t)
	if code := guardEngine("podman", []string{"ps"}); code != 0 {
		t.Fatal(code)
	}
	if len(*calls) != 1 || (*calls)[0][0] != (coord.Use{Kind: "sandbox", Name: "bashy"}) {
		t.Fatalf("direct guardEngine calls = %v, want one sandbox:bashy", *calls)
	}
}

func TestClaimGuardSkipsRegistryLookupForShippedVerbs(t *testing.T) {
	setupIsolatedCoord(t)
	countGuards(t)
	lookups := countRegisteredLookups(t)
	mw := claimGuardMiddleware(func(context.Context, []string) error { return nil })

	for _, argv := range [][]string{
		{"ls"}, {"cat", "x"}, {"echo", "hi"}, {"git", "status"}, {"bashy", "git", "log"},
		{"podman", "ps"}, {"docker", "ps"}, {"sandbox", "ps"}, {"ollama", "list"},
	} {
		if err := mw(context.Background(), argv); err != nil {
			t.Fatal(err)
		}
	}
	if *lookups != 0 {
		t.Errorf("registry lookups for shipped verbs = %d, want 0", *lookups)
	}

	if err := mw(context.Background(), []string{"not-a-shipped-command-xyz"}); err != nil {
		t.Fatal(err)
	}
	if *lookups != 1 {
		t.Errorf("registry lookups for an unknown verb = %d, want 1", *lookups)
	}
}

func BenchmarkClaimGuardMiddlewareShippedVerb(b *testing.B) {
	b.Setenv("CLAUDECODE", "1")
	b.Setenv("BASHY_COORD_DIR", b.TempDir())
	mw := claimGuardMiddleware(func(context.Context, []string) error { return nil })
	argv := []string{"ls", "-l"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = mw(context.Background(), argv)
	}
}
