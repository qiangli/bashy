package agentos

import (
	"testing"

	"github.com/qiangli/coreutils/pkg/ref"
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
