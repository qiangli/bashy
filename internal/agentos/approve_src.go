package agentos

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/ask"
	"github.com/qiangli/yoke/pkg/src"
)

// approveSrcForm is the argv shape an agent raises through the existing
// ask/approve human channel before calling `approve src`:
//
//	approve request --source confirm -- src protected-edit <repo> <path...>
//
// The argv binds the human's answer to the exact edit: the token is the
// hash of this argv, so an allow applies to these paths and no others.
var approveSrcForm = []string{"src", "protected-edit"}

// humanVias are the ask decision channels that count as a recorded human
// answer. ViaTimeout is a denial by construction (Allowed is false), so it
// can never pass the gate below; anything outside this set is refused.
var humanVias = map[string]bool{
	ask.ViaHuman:  true,
	ask.ViaReview: true,
	ask.ViaChat:   true,
	ask.ViaACP:    true,
}

func newApproveSrcCmd() *cobra.Command {
	var paths []string
	var approvalID, reason string
	cmd := &cobra.Command{
		Use:   "src <repo>",
		Short: "Record a protected-edit approval after a human allowed it (never self-approve)",
		Long: `Record a protected-edit approval for a src repo's guard-rail paths.

The agent first raises the edit through the human channel:

  bashy approve request --source confirm -- src protected-edit <repo> <path...>

A human allows it (bashy approve allow ID, a chat grant, or ACP), and only
then does this form write the pkg/src ApprovalRecord a protected PR merge
requires:

  bashy approve src <repo> --path <path> [--path ...] --approval ID [--reason R]

It refuses when no recorded human allow decision covers exactly this
repo and these paths: an agent cannot self-approve. Core paths are always
refused — no approval can unlock them. Skill-tier paths need no approval
and are refused here so meaningless records are never written.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := approveSrcRecord(args[0], paths, approvalID, reason)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "recorded approval %s\n", id)
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&paths, "path", nil, "protected repo-relative path to approve (repeatable)")
	cmd.Flags().StringVar(&approvalID, "approval", "", "ask approval ID carrying the recorded human answer (required)")
	cmd.Flags().StringVar(&reason, "reason", "", "why (recorded on the approval)")
	return cmd
}

// approveSrcRecord verifies the recorded human answer and writes the
// pkg/src ApprovalRecord. It never writes a decision itself: without a
// recorded allow through the human channel it refuses, so an agent cannot
// self-approve through this form.
func approveSrcRecord(repo string, paths []string, approvalID, reason string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("approve src: at least one --path is required")
	}
	seen := map[string]bool{}
	deduped := paths[:0]
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			return "", fmt.Errorf("approve src: empty --path")
		}
		if !seen[p] {
			seen[p] = true
			deduped = append(deduped, p)
		}
	}
	paths = deduped
	if strings.TrimSpace(approvalID) == "" {
		return "", fmt.Errorf("approve src: --approval is required: raise `approve request -- src protected-edit %s <paths>` and have a human allow it (an agent cannot self-approve)", repo)
	}
	r, err := ask.Find(approvalID)
	if err != nil {
		return "", fmt.Errorf("approve src: %w", err)
	}
	if !r.IsApproval() || r.Approval == nil {
		return "", fmt.Errorf("approve src: %s is a value request, not an approval — the human answer must come through the approve channel", approvalID)
	}
	d, ok := ask.LoadDecision(r.ID)
	if !ok {
		return "", fmt.Errorf("approve src: no recorded human answer on approval %s (an agent cannot self-approve)", approvalID)
	}
	if !d.Allowed() {
		return "", fmt.Errorf("approve src: approval %s was %s, not allowed (by %s)", approvalID, d.Verdict, d.By)
	}
	if !humanVias[d.Via] {
		return "", fmt.Errorf("approve src: approval %s was decided via %q, not a human channel (an agent cannot self-approve)", approvalID, d.Via)
	}
	if err := approveSrcArgvMatches(r.Approval.Argv, repo, paths); err != nil {
		return "", err
	}
	var core, prot []string
	for _, p := range paths {
		switch src.PathClass(repo, p) {
		case src.ClassCore:
			core = append(core, p)
		case src.ClassProtected:
			prot = append(prot, p)
		}
	}
	if len(core) > 0 {
		return "", fmt.Errorf("approve src: core path(s) %s are immutable (no approval can unlock them)", strings.Join(core, ", "))
	}
	if len(prot) == 0 {
		return "", fmt.Errorf("approve src: no protected paths among %s: skill-tier edits merge without approval", strings.Join(paths, ", "))
	}
	approver := strings.TrimSpace(d.By)
	if approver == "" {
		approver = "human"
	}
	if strings.TrimSpace(reason) == "" {
		reason = "human approval " + r.ID + " (" + d.Via + ")"
	}
	return src.WriteApproval("", src.ApprovalRecord{
		Repo: repo, Paths: append([]string(nil), paths...),
		Approver: approver, Reason: reason,
	})
}

// approveSrcArgvMatches requires the recorded approval's argv to name
// exactly this repo and these paths under the protected-edit form, so a
// human allow for one edit cannot authorize another. Paths compare as a
// set; the form head and repo compare positionally.
func approveSrcArgvMatches(argv []string, repo string, paths []string) error {
	if len(argv) < 3 || argv[0] != approveSrcForm[0] || argv[1] != approveSrcForm[1] || argv[2] != repo {
		return mismatchedEdit(argv, repo, paths)
	}
	got := map[string]bool{}
	for _, p := range argv[3:] {
		got[p] = true
	}
	if len(got) != len(paths) {
		return mismatchedEdit(argv, repo, paths)
	}
	for _, p := range paths {
		if !got[p] {
			return mismatchedEdit(argv, repo, paths)
		}
	}
	return nil
}

func mismatchedEdit(argv []string, repo string, paths []string) error {
	return fmt.Errorf("approve src: recorded approval covers %q, not `src protected-edit %s %s` (an allow applies to the exact edit raised)", strings.Join(argv, " "), repo, strings.Join(paths, " "))
}
