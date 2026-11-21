package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

func wireMessageBoard() {
	bus.FleetNames = func() []string {
		agents, _ := fleet.New().Agents()
		out := make([]string, 0, len(agents))
		for _, a := range agents {
			if n := strings.TrimSpace(a.Name); n != "" {
				out = append(out, n)
			}
		}
		return out
	}
	bus.DetectHarness = fleet.DetectTool
	bus.CurrentSessionClaim = func(agentName string) string {
		agent, ok := fleet.New().Agent(agentName)
		if !ok {
			return ""
		}
		tool, ok := fleet.New().Tool(agent.Tool)
		if !ok {
			return ""
		}
		return tool.CurrentSession()
	}
}

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
				}
			}
			hint := "take the seat with 'bashy sprint claim last <story> --owner " + as + "'"
			if holderPID > 0 {
				if holderMode == "foreman" {
					hint = fmt.Sprintf("foreman pid %d owns this seat; inspect with 'bashy foreman status' or release with 'bashy foreman pause'", holderPID)
				} else {
					hint = fmt.Sprintf("pid %d (%s) owns this seat; release it there or take explicitly", holderPID, holderMode)
				}
			}
			return fmt.Errorf("%w (%s; %s)", err, holderDetail, hint)
		}
	}
}

func main() {
	wireMessageBoard()
	cmd := bus.NewMessageBoardCmdWithSprintResolver(weave.ResolveSprintScopeByHandle)
	wrapAuthoredClaimErrors(cmd)
	os.Exit(adapter.RunCobra(cmd, "bashy mb:", func(err error) int {
		var coded interface{ ExitCode() int }
		if errors.As(err, &coded) {
			return coded.ExitCode()
		}
		return bus.ExitCode(err)
	}))
}
