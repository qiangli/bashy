package agentos

import (
	"sync"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/execlog"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/kb"
	"github.com/qiangli/yoke/pkg/lexicon"
	"github.com/qiangli/yoke/pkg/meet"
	"github.com/qiangli/yoke/pkg/principal"
	"github.com/qiangli/yoke/pkg/ref"
	"github.com/qiangli/yoke/pkg/todo"
	"github.com/qiangli/yoke/pkg/weave"
)

// wireRefResolvers fills lexicon.RefResolvers — the registry `bashy define
// <kind>:<id>` asks — with one resolver per kind in the ref vocabulary, each
// from the store that owns the kind. This is the ONE place that sees every
// store (dhnt docs/uniform-ref-addressing.md D5): pkg/lexicon imports none of
// them, pkg/kb resolves only kb, and the stores import each other in a partial
// order, so the shell is the only layer below nothing.
//
// The registrations pass exactly what each store's own CLI passes with no
// flags — the cwd's repo store for kb and todo, the default fleet catalog, the
// default exec-history root — so `define todo:abc` and `todo show abc` agree
// on which store they read. A scoped ref (`kb:<repo>/<handle>`,
// `todo:<repo>/<seq>`) resolves its scope segment through ONE lookup,
// weave.ScopeLookup() — the cwd checkout first, then every weave queue root
// by basename, ambiguity an error naming both — built once here and handed to
// both stores, so kb and todo agree on which checkout a basename names.
//
// The contract this protects: a hook left nil is indistinguishable from a
// store that is empty (the wireLexicon comment records the day that drift was
// silent). define reports an unregistered kind as "no resolver wired", not as
// "unknown", and TestRefResolversCoverEveryKind fails the build the day a kind
// joins the vocabulary without a line here.
func wireRefResolvers() {
	refResolversOnce.Do(func() {
		g := lexicon.RefResolvers
		scopes := weave.ScopeLookup()
		kb.RegisterRefs(g, "", scopes)
		todo.RegisterRefs(g, "", false, false, "", scopes)
		weave.RegisterRefs(g)       // sprint, run
		meet.RegisterRefs(g)        // meet
		bus.RegisterRefs(g)         // mb, bus
		fleet.RegisterRefs(g)       // agent tool model skill host
		principal.RegisterRefs(g)   // person role
		execlog.RegisterRefs(g, "") // episode
	})
}

var refResolversOnce sync.Once

// unwiredRefKinds is what the coverage test asserts empty; exposed as a
// function so the test reads the same registry define does.
func unwiredRefKinds() []ref.Kind {
	wireRefResolvers()
	return lexicon.RefResolvers.Missing()
}
