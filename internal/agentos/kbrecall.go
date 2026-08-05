package agentos

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/recall"
)

// newKBRecallCmd mounts the cross-ring read surface (coreutils/pkg/recall) as
// `bashy kb recall`.
//
// # Why it lives here and not in pkg/kb
//
// pkg/recall imports pkg/kb (its host ring wraps kb.Store), and pkg/kb is
// pinned as an import LEAF by pkg/fleet/leaf_test.go. Registering the
// subcommand inside NewKBCmd would invert that and close an import cycle, so
// the composition happens at the dispatch layer, where bashy already imports
// both. pkg/kb stays a leaf and pkg/recall stays independently consumable by
// tools that are not bashy.
//
// # Why it is under `kb` at all
//
// It was a top-level `bashy recall` until 2026-08-05. Four days of telemetry
// (61,084 dispatched commands) recorded exactly one agent reaching for this
// capability unprompted, and it typed `bashy kb recall` — the verb did not
// exist there, so it exited 1 and the agent never tried again. The mental model
// in evidence is that memory lives under one noun. Moving it costs nothing:
// over the same window `bashy recall` had zero uses outside its own
// development, so the interface commitment in dhnt/docs/bashy-recall-spec.md
// had no dependents to break. It would not stay free.
//
// The three neighbours keep their distinct questions — `craft` answers "how do
// I do X" and composes; `kb search` administers ONE ring and knows about slugs
// and page status; `recall` only READS, across every ring, and never composes.
// Nesting changes the path, not the contract.
func newKBRecallCmd() *cobra.Command {
	cmd := recall.NewRecallCmd()

	// kb's own PersistentPreRunE resolves ONE store and prints a `kb [scope]
	// <path>` header. That is meaningless for recall, which reads every ring
	// and has no single store, and the header would land on the stderr of a
	// surface third parties parse. Cobra runs only the closest hook, so
	// defining one here suppresses kb's.
	//
	// The inherited scope flags are REFUSED rather than ignored. Accepting
	// `--repo` and then searching every ring anyway would be a silent lie about
	// what was read — the failure mode this tree names absence-of-evidence.
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		for _, f := range []string{"dir", "repo", "user", "base-dir"} {
			if fl := c.Flags().Lookup(f); fl != nil && fl.Changed {
				return fmt.Errorf(
					"--%s selects ONE kb store; recall reads every memory ring, so it cannot honour it — use --store <root> to move the whole set", f)
			}
		}
		return nil
	}
	return cmd
}
