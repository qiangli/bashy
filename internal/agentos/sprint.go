package agentos

import (
	"strings"

	"github.com/qiangli/coreutils/pkg/weave"
	"github.com/spf13/cobra"
)

// newSprintCmd wraps weave.NewSprintCmd (the cross-repo plan/handoff board) and
// makes the OWNER-ACCOUNTABILITY contract part of `bashy sprint --help` itself.
//
// The plan/handoff layer describes cards, leases, and continuity records — the
// MECHANICS. Left at that, the help reads as though holding a sprint's conductor
// lease were a passive claim: take the lease, checkpoint, done. It is not. An
// appointed sprint owner/conductor is ACTIVE and accountable for the sprint
// through verified completion, not a lease holder waiting to be pulled from. That
// requirement is easy for a human to state once and lose; wiring it into the
// command's own help makes it durable — every agent that reads `sprint --help`
// sees the accountability contract next to the mechanics it is the point of.
func newSprintCmd() *cobra.Command {
	cmd := weave.NewSprintCmd()
	cmd.Short = "Plan/handoff board AND the ACTIVE owner-accountability contract for a sprint"
	if !strings.Contains(cmd.Long, ownerAccountabilityHelp) {
		cmd.Long = strings.TrimRight(cmd.Long, "\n") + "\n\n" + ownerAccountabilityHelp
	}
	return cmd
}

// ownerAccountabilityHelp is the active-owner contract appended to
// `bashy sprint --help`. It is exported through the help of the command an agent
// reads before driving a sprint, so the accountability requirement travels with
// the tool rather than living only in a human's memory. The conductor skill
// (`bashy skills show conductor`) carries the same checklist in depth.
const ownerAccountabilityHelp = `OWNER ACCOUNTABILITY — an appointed owner/conductor is not a passive lease holder.

Holding a sprint's conductor lease makes you ACCOUNTABLE for it through VERIFIED
completion — not merely the recorded claimant. While you hold it you must:

  - Stay REACHABLE: keep a live identity and watch your inbox and host activity
    (` + "`bashy inbox --as <you>`" + `); process human instructions first each turn.
  - SUPERVISE workers proactively: monitor every run, and steer, interrupt,
    salvage, or reassign stalled / failed / no-op work instead of waiting on it.
  - VERIFY independently: inspect and rerun the gates yourself; never trust a
    worker's prose or exit status ("submitted" and exit 0 are not "done").
  - INTEGRATE sequentially: serialize reviews and merges; push subproject commits
    and bump parent pins per repository policy, one gated step at a time.
  - PRESERVE + clean up within bounds: leave unrelated work untouched; clean the
    workspaces, branches, temp state, and disposable resources YOU integrated —
    only when authorized, and never beyond the assignment or the safety policy.
  - Keep CONTINUITY: checkpoint often so any handoff or crash resumes cleanly.

Do not wait passively while actionable work remains. Cleanup and deletion
authority is bounded by the assignment and the safety policy — it does not extend
to unrelated repositories, branches, or resources.`
