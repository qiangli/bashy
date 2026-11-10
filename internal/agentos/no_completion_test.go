// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// Sprint: #606; Story: #74; Story-ID: 0ada8418d6e

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestNoCobraCompletionCommand walks every command tree bashy mounts and
// proves none of them carries cobra's default `completion` subcommand.
//
// Generated completion scripts are for old-fashioned shells; bashy does
// discovery through its own surface (--skill, help --format=mcp, the
// manifest, hints, and the shell's built-in completion from command
// metadata). The policy lives in ONE place (disableCobraCompletion, applied
// by the setCobraArgs chokepoint and directly at the few dispatch sites
// that set argv without it), and this test is its ratchet: each tree is
// mounted exactly as dispatch mounts it, put through the chokepoint, given
// the same Execute-time initialization cobra performs, and then walked in
// full. A tree that bypasses the chokepoint without the helper fails here.
func TestNoCobraCompletionCommand(t *testing.T) {
	ringDir(t)

	names := map[string]struct{}{}
	_, _, verbs := commandsCatalog()
	for _, v := range verbs {
		names[v] = struct{}{}
	}
	for _, v := range hiddenVerbsCatalog() {
		names[v] = struct{}{}
	}
	for _, v := range dispatchOnlyNames {
		names[v] = struct{}{}
	}
	for name := range cobraOptionalVerbs {
		names[name] = struct{}{}
	}

	// Verbs at the heart of S606 must be walked, not skipped: if one of
	// these ever stops resolving to a cobra tree, the walk below would go
	// green by covering nothing.
	mustWalk := []string{
		"satellite", "fleet", "member", "paired-host", "token", "reproduce",
		"llm", "adapt", "skill", "kb", "weave", "sprint", "meet", "chat",
		"ask", "approve", "steward", "conductor",
	}
	must := map[string]bool{}
	for _, m := range mustWalk {
		must[m] = false
	}

	walked, skipped := 0, 0
	for name := range names {
		cmd := cobraVerb(name)
		if cmd == nil {
			skipped++
			continue // hand-parsed front door or external exec: no cobra tree
		}
		if _, ok := must[name]; ok {
			must[name] = true
		}
		// The same mount dispatch runs: the ONE place under test.
		setCobraArgs(cmd, nil)
		// What Execute does on the root before dispatching.
		cmd.InitDefaultCompletionCmd()
		var found []string
		var walk func(c *cobra.Command, path string)
		walk = func(c *cobra.Command, path string) {
			for _, sub := range c.Commands() {
				p := path + " " + sub.Name()
				if sub.Name() == "completion" || sub.HasAlias("completion") {
					found = append(found, p)
				}
				walk(sub, p)
			}
		}
		walk(cmd, name)
		if len(found) > 0 {
			t.Errorf("%s exposes cobra completion: %v", name, found)
		}
		walked++
	}
	for name, seen := range must {
		if !seen {
			t.Errorf("must-walk verb %q resolved to no cobra tree", name)
		}
	}
	if walked == 0 {
		t.Fatal("no command trees walked")
	}
	t.Logf("walked %d command trees (%d hand-parsed names skipped)", walked, skipped)
}
