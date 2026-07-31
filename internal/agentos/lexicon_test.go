// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/lexicon"
)

// THE BUG THIS PINS. `bashy lexicon study` collects the host's addresses and
// hands each one to lexicon.RecordDiscovery so it reaches the host-local fact
// store. Only the `define` dispatch arm set that hook, and `study` lives under
// `lexicon` — so the command printed "N identity value(s) found; no fact store
// wired, nothing recorded" and exited 0. A success state reached by the ABSENCE
// of a hook: nothing failed, nothing was stored, and the only evidence was a
// line of prose on a path nobody reads twice.
//
// Both entry points now go through wireLexicon, and this asserts each one
// leaves the package fully configured rather than trusting that they call it.
func TestLexiconEntryPointsWireTheFactStore(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func() any
	}{
		{"lexicon", func() any { return newLexiconCmd() }},
		{"define", func() any { return newDefineCmd() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lexicon.RecordDiscovery = nil
			lexicon.Synopses = nil
			lexicon.KnownCommands = nil

			_ = tc.build()

			if lexicon.RecordDiscovery == nil {
				t.Error("RecordDiscovery is nil — collection will report its findings and " +
					"silently store none")
			}
			if len(lexicon.Synopses) == 0 {
				t.Error("Synopses is empty — every verb would resolve without its one-liner")
			}
			if len(lexicon.KnownCommands) == 0 {
				t.Error("KnownCommands is empty — the standard command set would not be " +
					"subtracted, so every standard tool would be reported as local jargon")
			}
		})
	}
}

// `bashy define` must never gain a subcommand.
//
// Its argument is an arbitrary user token, so every subcommand name permanently
// removes a word from the definable vocabulary: mount `study` here and
// `bashy define study` stops meaning "what is the word study" and starts
// printing a help screen. The failure is invisible until somebody asks about
// that exact word.
//
// pkg/lexicon ratchets its own constructor; this ratchets the MOUNT, which is
// the half a coreutils test cannot see — bashy owns the dispatch arm and could
// add a subcommand to the returned command without touching that package.
func TestDefineCmdHasNoSubcommands(t *testing.T) {
	subs := newDefineCmd().Commands()
	if len(subs) == 0 {
		return
	}
	names := make([]string, 0, len(subs))
	for _, c := range subs {
		names = append(names, c.Name())
	}
	t.Fatalf("bashy define has subcommands [%s] — each one steals that word from the "+
		"definable vocabulary; mount the action under `bashy lexicon` instead",
		strings.Join(names, " "))
}
