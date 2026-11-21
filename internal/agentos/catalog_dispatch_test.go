// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/qiangli/bashy/internal/cmdgen"
	"github.com/qiangli/yoke/pkg/shape/slot"
)

func withCatalogDispatchSeams(t *testing.T) {
	t.Helper()
	withShapeSlotSeams(t)
	oldLookup, oldEnsure := catalogCommandLookup, catalogCommandEnsureSlot
	t.Cleanup(func() {
		catalogCommandLookup = oldLookup
		catalogCommandEnsureSlot = oldEnsure
	})
	t.Setenv("BASHY_CMD_DISPATCH", "")
}

func TestCatalogDispatchUsesActiveShapeSlot(t *testing.T) {
	withCatalogDispatchSeams(t)
	activePath := filepath.Join(t.TempDir(), "stats")
	catalogCommandLookup = func(name string) (cmdgen.Entry, bool) {
		if name == "stats" {
			return cmdgen.Entry{Name: "stats", Kind: cmdgen.KindCobra, Module: "m", Package: "m/p", Constructor: "NewCmd"}, true
		}
		return cmdgen.Entry{}, false
	}
	shapeSlotApp = func(name string) bool { return name == "stats" }
	shapeSlotSelect = func(app string) (slot.Version, bool, error) {
		if app != "stats" {
			t.Fatalf("app = %q, want stats", app)
		}
		return slot.Version{Path: activePath}, true, nil
	}
	catalogCommandEnsureSlot = func(context.Context, cmdgen.Entry) (slot.Version, error) {
		t.Fatal("EnsureSlot must not run when an active slot is already selected")
		return slot.Version{}, nil
	}
	var gotPath string
	var gotArgs []string
	var hasMarker bool
	shapeSlotLaunch = func(path string, args, env []string) int {
		gotPath = path
		gotArgs = append([]string(nil), args...)
		for _, kv := range env {
			if kv == slot.EntryEnvironmentKey+"="+activePath {
				hasMarker = true
			}
		}
		return 3
	}

	code, handled := dispatchCatalogCommand(context.Background(), "stats", []string{"summary", "--json"})
	if !handled || code != 3 {
		t.Fatalf("dispatch = (%d, %v), want (3, true)", code, handled)
	}
	if gotPath != activePath {
		t.Fatalf("launched path = %q, want %q", gotPath, activePath)
	}
	wantArgs := []string{"stats", "summary", "--json"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("launched args = %v, want %v", gotArgs, wantArgs)
	}
	if !hasMarker {
		t.Fatal("launch env missing BASHY_SHAPE_ENTRY marker")
	}
}

func TestCatalogDispatchBuildsOnDemandWhenSlotEmpty(t *testing.T) {
	withCatalogDispatchSeams(t)
	builtPath := filepath.Join(t.TempDir(), "stats-built")
	catalogCommandLookup = func(name string) (cmdgen.Entry, bool) {
		if name == "stats" {
			return cmdgen.Entry{Name: "stats", Kind: cmdgen.KindCobra, Module: "m", Package: "m/p", Constructor: "NewCmd"}, true
		}
		return cmdgen.Entry{}, false
	}
	shapeSlotApp = func(name string) bool { return name == "stats" }
	selectCalls := 0
	shapeSlotSelect = func(app string) (slot.Version, bool, error) {
		selectCalls++
		if selectCalls == 1 {
			return slot.Version{}, false, nil
		}
		return slot.Version{Path: builtPath}, true, nil
	}
	ensureCalled := false
	catalogCommandEnsureSlot = func(_ context.Context, e cmdgen.Entry) (slot.Version, error) {
		ensureCalled = true
		return slot.Version{Path: builtPath}, nil
	}
	var launched string
	shapeSlotLaunch = func(path string, args, env []string) int {
		launched = path
		return 0
	}

	code, handled := dispatchCatalogCommand(context.Background(), "stats", []string{"--help"})
	if !handled || code != 0 || !ensureCalled || launched != builtPath {
		t.Fatalf("code=%d handled=%v ensureCalled=%v launched=%q", code, handled, ensureCalled, launched)
	}
}

func TestCatalogDispatchRespectsCertAndInprocessOverride(t *testing.T) {
	withCatalogDispatchSeams(t)
	catalogCommandLookup = func(string) (cmdgen.Entry, bool) {
		t.Fatal("lookup must not run when disabled")
		return cmdgen.Entry{}, false
	}

	t.Setenv("VSC_PROFILE", "cert")
	if _, handled := dispatchCatalogCommand(context.Background(), "stats", nil); handled {
		t.Fatal("dispatched under VSC_PROFILE=cert")
	}

	t.Setenv("VSC_PROFILE", "")
	t.Setenv("BASHY_CMD_DISPATCH", "inprocess")
	if _, handled := dispatchCatalogCommand(context.Background(), "stats", nil); handled {
		t.Fatal("dispatched under BASHY_CMD_DISPATCH=inprocess")
	}
}

func TestCatalogDispatchReportsBuildFailure(t *testing.T) {
	withCatalogDispatchSeams(t)
	catalogCommandLookup = func(name string) (cmdgen.Entry, bool) {
		if name == "broken" {
			return cmdgen.Entry{Name: "broken", Kind: cmdgen.KindCobra, Module: "m", Package: "m/p", Constructor: "NewCmd"}, true
		}
		return cmdgen.Entry{}, false
	}
	shapeSlotSelect = func(string) (slot.Version, bool, error) { return slot.Version{}, false, nil }
	catalogCommandEnsureSlot = func(context.Context, cmdgen.Entry) (slot.Version, error) {
		return slot.Version{}, errors.New("compile failed")
	}
	code, handled := dispatchCatalogCommand(context.Background(), "broken", nil)
	if !handled || code != 126 {
		t.Fatalf("code=%d handled=%v, want (126, true)", code, handled)
	}
}
