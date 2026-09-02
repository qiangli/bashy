package agentos

import (
	"fmt"
	"io"
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
	cmd.AddCommand(newSprintInboxAckCmd())
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
		writeSprintWatchNextSteps(cmd.ErrOrStderr(), id, owner)
		poll := defaultInboxPollRuntime(true)
		poll.ownerLive = claim.ownerLive
		rt := defaultSprintWatchRuntime()
		rt.poll = poll
		return runSprintInboxWatch(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), id, owner, rt)
	}
}

func sprintWatchParentPID() int { return os.Getppid() }

// writeSprintWatchNextSteps tells the agent what it must now DO, at the moment
// it attaches.
//
// The accountability contract is already in `sprint --help`, and that is not
// the same thing: help is read once, before the work, by an agent that does not
// yet know which of it will matter. This prints at the one moment the loop
// becomes the agent's responsibility, with the sprint id and owner name already
// substituted so there is nothing to assemble.
//
// It exists because the omission was MEASURED, repeatedly, on this project's
// own conductor seat: an agent attached, read the banner "retain and read this
// process", did not know an explicit ack was owed, and lost the seat to the
// fail-closed fuse — twice in one session. The banner was true and insufficient.
// Nothing about the failure was visible until the process exited nonzero.
//
// Written to STDERR: stdout carries the NDJSON delivery stream, and a reader
// parsing it must not have to skip prose.
//
// Compact on purpose. Three steps, each with its consequence, is what an agent
// reliably acts on; a longer block is one an agent skims past, which is how the
// contract in `--help` came to be unread in the first place.
func writeSprintWatchNextSteps(w io.Writer, id int64, owner string) {
	fmt.Fprintf(w, `NEXT STEPS — this stream IS your inbox; nothing else will wake you.
  1. READ each NDJSON line printed below as it arrives. Do NOT start a second
     "bashy inbox --watch" for %[2]s: two readers race for the same cursors.
  2. After handling a batch, ACK it:  bashy sprint inbox-ack %[1]d --as %[2]s
     Unacked input is reminded every 3m; the 3rd reminder EXITS NONZERO with
     the mail still unread and the seat then ages out.
  3. If this process ends for ANY reason the seat goes UNREACHABLE. Re-attach:
     bashy sprint take %[1]d --as %[2]s --watch
`, id, owner)
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
    A Bashy-managed session receives input through its control transport. An external
    Claude/Codex/OpenCode/ycode/agy harness must claim with ` + "`sprint take/start --watch`" + `
    and retain/read that foreground process. After handling each delivered batch it must run
    ` + "`sprint inbox-ack ID --as OWNER`" + `. Unacknowledged input is reminded every three
    minutes; the third reminder exits nonzero with input still unread, requiring a rerun.
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
