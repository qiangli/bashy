package agentos

import (
	"fmt"
	"os"
	"strconv"
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
	for _, child := range cmd.Commands() {
		if child.Name() == "start" || child.Name() == "take" {
			attachSprintWatch(child, child.Name() == "take")
		}
	}
	return cmd
}

// attachSprintWatch lets an EXTERNAL agent harness own the conductor seat
// without pretending Bashy can inject a turn into a process it did not launch.
// The sprint command itself stays alive as the harness's foreground tool call;
// its parent relationship and canonical room claim are the delivery proof.
func attachSprintWatch(cmd *cobra.Command, takeover bool) {
	var watch bool
	cmd.Flags().BoolVar(&watch, "watch", false,
		"after claiming, stay attached and stream unified inbox NDJSON (required for an external agent harness)")
	original := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !watch {
			return original(cmd, args)
		}
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("sprint must be an integer: %q", args[0])
		}
		explicit, _ := cmd.Flags().GetString("as")
		owner, err := weave.SprintClaimIdentity(id, explicit, takeover)
		if err != nil {
			return err
		}
		claim, err := registerSprintInboxWatcher(owner)
		if err != nil {
			return fmt.Errorf("sprint %s --watch: %w", cmd.Name(), err)
		}
		defer claim.leave()
		if err := original(cmd, args); err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(),
			"sprint %s: attached inbox stream for %s (parent pid %d); retain and read this process until handoff\n",
			cmd.Name(), owner, sprintWatchParentPID())
		poll := defaultInboxPollRuntime(true)
		poll.ownerLive = claim.ownerLive
		return runUnifiedInboxWithPoll(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(),
			owner, 0, false, true, true, 0, poll)
	}
}

func sprintWatchParentPID() int { return os.Getppid() }

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
    A Bashy-managed session receives input through its control transport. An external
    Claude/Codex/OpenCode/ycode/agy harness must claim with ` + "`sprint take/start --watch`" + `
    and retain/read that foreground process; its parent relationship is the delivery proof.
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
