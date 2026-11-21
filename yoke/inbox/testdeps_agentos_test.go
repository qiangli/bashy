package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/feed"
	"github.com/qiangli/yoke/pkg/instance"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/weave"
)

// bashySelfPath is the bashy executable that shims and re-entries run
// (`git() { command <self> git "$@"; }`). It is the running binary only when
// that binary IS bashy: when bashy is embedded as a library (ycode's harness
// session, pkg/runner) os.Executable() is the host, and re-entering the host's
// CLI would hand bashy verbs to a different program. Order: an explicit
// BASHY_SELF from the embedder; the running executable when it is bashy (or a
// Go test binary, which stands in for it in this package's tests); bashy on
// PATH; else the bare name, so a missing bashy fails loudly instead of
// silently re-entering the host.
// RunningAsBashy is set by cmd/bashy: the running executable is a bashy
// build under any file name, so os.Executable() is the right self path.
var RunningAsBashy bool

func bashySelfPath() string {
	if self := strings.TrimSpace(os.Getenv("BASHY_SELF")); self != "" {
		return self
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		if len(os.Args) > 0 && os.Args[0] != "" {
			exe = os.Args[0]
		}
	}
	if exe != "" && (RunningAsBashy || isBashyExecutable(exe) || strings.HasSuffix(strings.TrimSuffix(filepath.Base(exe), ".exe"), ".test")) {
		return exe
	}
	if path, err := exec.LookPath("bashy"); err == nil {
		return path
	}
	return "bashy"
}

func isBashyExecutable(path string) bool {
	switch strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".exe") {
	case "bashy", "bashy.real":
		return true
	}
	return false
}

func newBusFrontDoorCmd(name string) (*cobra.Command, string, bool) {
	wireMessageBoard()
	switch name {
	case "mb", "messages":
		cmd := bus.NewMessageBoardCmdWithSprintResolver(weave.ResolveSprintScopeByHandle)
		wrapAuthoredClaimErrors(cmd)
		return cmd, "mb", true
	case "inbox":
		return newUnifiedInboxCmd(), "inbox", true
	case "notify":
		cmd := bus.NewNotifyCmd()
		wrapAuthoredClaimErrors(cmd)
		return cmd, "notify", true
	case "feed":
		return feed.NewCmd(), "feed", true
	case "instance":
		return instance.NewCmd(nil), "instance", true
	default:
		return nil, "", false
	}
}

// wrapAuthoredClaimErrors enhances "no matching live session claim" refusals
// with the conflicting holder and how to take the claim, so an attended
// session that is refused knows whether a foreman or another session holds
// the seat. When a live foreign holder exists the peek does not hijack;
// the operator must take the seat explicitly.
func wrapAuthoredClaimErrors(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		orig := sub.RunE
		if orig == nil {
			continue
		}
		sub.RunE = func(c *cobra.Command, args []string) error {
			err := orig(c, args)
			if err == nil || !strings.Contains(err.Error(), "no matching live session claim") {
				return err
			}
			as, _ := c.Flags().GetString("as")
			if strings.TrimSpace(as) == "" {
				as, _ = c.Flags().GetString("from")
			}
			// Try to resolve canonical name for holder lookup.
			holderDetail := ""
			holderMode := ""
			holderPID := 0
			if canon := strings.TrimSpace(as); canon != "" {
				if card, live, ferr := room.Find(room.AgentClaimID(canon)); ferr == nil && live {
					mode := strings.TrimSpace(card.Mode)
					if mode == "" {
						mode = "unknown"
					}
					holderMode = mode
					owner := card.OwnerPID
					if owner == 0 {
						owner = card.PID
					}
					holderPID = owner
					holderDetail = fmt.Sprintf(" holder: pid %d, mode %q, task %q, cwd %q, session %q", owner, mode, strings.TrimSpace(card.Task), strings.TrimSpace(card.Cwd), card.SessionClaim)
				} else if card, live, ferr := room.Find(canon); ferr == nil && live {
					holderDetail = fmt.Sprintf(" holder: pid %d, mode %q", card.PID, strings.TrimSpace(card.Mode))
					holderMode = strings.TrimSpace(card.Mode)
					holderPID = card.PID
				}
			}
			if holderPID != 0 && holderMode != "" {
				return fmt.Errorf("%w; seat %q is held live by %s pid %d in another session;%s take it with `bashy sprint take` / ask that session to release it, or run `bashy inbox --as %s --peek` in the SAME session only when the seat is stale or you already hold it", err, as, holderMode, holderPID, holderDetail+";", as)
			}
			extra := ""
			if holderDetail != "" {
				extra = holderDetail + ";"
			}
			return fmt.Errorf("%w; seat %q is held by another live session.%s take the claim by running `bashy inbox --as %s --peek` (or `--watch`) in the SAME session (same CLAUDE_CODE_SESSION_ID / session key) before `bashy mb send --as %s ...`, or export BASHY_AGENT=%s when the seat is unclaimed", err, as, extra, as, as, as)
		}
	}
	// Also wrap the parent command itself (e.g. `bashy mb --as X` read)
	if cmd.RunE != nil {
		orig := cmd.RunE
		cmd.RunE = func(c *cobra.Command, args []string) error {
			err := orig(c, args)
			if err == nil || !strings.Contains(err.Error(), "no matching live session claim") {
				return err
			}
			return err
		}
	}
}
