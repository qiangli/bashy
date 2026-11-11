package agentos

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	lenscmd "github.com/qiangli/yoke/cmds/lens"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/lens/provider"
	"github.com/qiangli/yoke/pkg/lexicon"
	"github.com/qiangli/yoke/pkg/ref"
)

// TestRefResolversCoverEveryKind is the coverage ratchet for `bashy define
// <kind>:<id>`: every kind in the ref vocabulary has a resolver registered by
// wireRefResolvers. Without this, adding a kind to pkg/ref compiles, its
// prose links parse, and define answers "no resolver wired on this build" —
// true, loud, and a regression nobody would notice until an agent hit it.
func TestRefResolversCoverEveryKind(t *testing.T) {
	if missing := unwiredRefKinds(); len(missing) != 0 {
		t.Fatalf("kinds with no resolver wired in wireRefResolvers: %v (vocabulary: %v)", missing, ref.KindNames())
	}
}

func TestAppRefResolverUsesConfiguredRegisteredCommands(t *testing.T) {
	registered := []fleet.Command{{Name: "notepad"}, {Name: "viewer"}}
	called := 0
	resolver := appRefResolver(
		func() []string { return []string{"notepad", "form"} },
		func() []fleet.Command { return registered },
		func(command, id string) ([]byte, error) {
			called++
			if command != "notepad" || id != "item-1" {
				t.Errorf("runner called for %s/%s", command, id)
			}
			return []byte(`{"title":"Draft"}`), nil
		},
	)
	if node, err := resolver.Resolve("notepad/item-1"); err != nil || node.Ref != "app:notepad/item-1" || node.Title != "Draft" {
		t.Fatalf("allowed app: node=%+v err=%v", node, err)
	}
	for _, id := range []string{"viewer/item-1", "form/item-1"} {
		if _, err := resolver.Resolve(id); !errors.Is(err, ref.ErrNotFound) {
			t.Errorf("%s: want unallowed/unregistered error, got %v", id, err)
		}
	}
	if called != 1 {
		t.Fatalf("runner calls = %d, want 1", called)
	}
	registered = nil // the ring is read again, so removal takes effect.
	if _, err := resolver.Resolve("notepad/item-1"); !errors.Is(err, ref.ErrNotFound) {
		t.Fatalf("removed command: %v", err)
	}
	denied := errors.New("access denied by app")
	resolver = appRefResolver(
		func() []string { return []string{"notepad"} },
		func() []fleet.Command { return []fleet.Command{{Name: "notepad"}} },
		func(_, _ string) ([]byte, error) { return nil, denied },
	)
	if _, err := resolver.Resolve("notepad/item-1"); !errors.Is(err, denied) {
		t.Fatalf("runner error lost: %v", err)
	}
}

func TestRefOutResolvesThroughWiredRegistry(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	store, err := shellOutputStore()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := store.Put([]byte("spilled bytes"))
	if err != nil {
		t.Fatal(err)
	}
	refResolversOnce = sync.Once{}
	t.Cleanup(func() { refResolversOnce = sync.Once{} })
	wireRefResolvers()

	handle := strings.TrimPrefix(digest, "sha256:")[:12]
	n, err := lexicon.RefResolvers.Resolve("out:" + handle)
	if err != nil {
		t.Fatalf("Resolve(out:%s): %v", handle, err)
	}
	if n.Kind != ref.Out {
		t.Errorf("kind = %v, want out", n.Kind)
	}
	if _, err := lexicon.RefResolvers.Resolve("out:000000000000"); err == nil {
		t.Error("unknown out handle resolved; want not-found")
	}
}

func TestLensAgentWorldUsesWiredRegistry(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	refResolversOnce = sync.Once{}
	t.Cleanup(func() { refResolversOnce = sync.Once{} })
	wireRefResolvers()

	if lenscmd.AgentWorldRefs != lexicon.RefResolvers {
		t.Fatal("lens AgentWorldRefs is not the wired lexicon.RefResolvers registry")
	}
	loc, err := ref.ParseLocator("sprint:999999999")
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.NewEntity(lenscmd.AgentWorldRefs, "sprint").Stat(context.Background(), loc)
	if err == nil {
		t.Fatal("absent sprint resolved")
	}
	if errors.Is(err, ref.ErrNoResolver) {
		t.Fatalf("wired sprint kind reported no resolver: %v", err)
	}
	if !errors.Is(err, ref.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
