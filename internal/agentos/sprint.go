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
completion and delivery from START TO END — not merely the recorded claimant.
Delegation transfers execution, never accountability. While you hold it you must:

  - Stay REACHABLE with the OWNER IDENTITY: a takeover uses the existing sprint owner name for
    mb/Meet/chat/ping, or explicitly updates ownership before using another name.
    Run as a Bashy-managed agent session under that exact name. Its harness automatically
    injects unified inbox input into real agent turns. A terminal ` + "`bashy inbox --watch`" + `
    only prints updates; it cannot wake an external Claude/Codex/OpenCode/ycode/agy harness.
  - SUPERVISE workers proactively: monitor every run, and steer, interrupt,
    salvage, or reassign stalled / failed / no-op work instead of waiting on it.
  - VERIFY independently: inspect and rerun the gates yourself; never trust a
    worker's prose or exit status ("submitted" and exit 0 are not "done").
  - INTEGRATE sequentially: serialize reviews and merges; push subproject commits
    and bump parent pins per repository policy, one gated step at a time.
  - COORDINATE OWNERSHIP: before touching work held by another sprint manager,
    contact that owner through mb/Meet/chat/ping and request sequencing or merge.
  - PRESERVE + clean up within bounds: proactively reclaim branches, git worktrees,
    weave workspaces, temp state, and disposable resources THIS sprint owns after
    integration. NEVER delete, remove, reset, overwrite, or destroy work owned by
    another sprint/agent. A coordination reply is not deletion authority.
  - Keep CONTINUITY: checkpoint often so any handoff or crash resumes cleanly.

Do not wait passively while actionable work remains. Cleanup and deletion
authority is bounded by the assignment and the safety policy — it does not extend
to unrelated repositories, branches, or resources.`
