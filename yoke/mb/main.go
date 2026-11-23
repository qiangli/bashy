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
	bus.CurrentSessionClaim = currentAgentSession
	bus.IsMainLineTool = isSteerToolName
}

func currentAgentSession(agentName string) string {
	if agent, ok := fleet.New().Agent(agentName); ok {
		if tool, ok := fleet.New().Tool(agent.Tool); ok {
			return tool.CurrentSession()
		}
	}
	if id, isInst := bus.ExplicitInstanceID(agentName); isInst {
		if inst, err := fleet.NewInstanceStore("").Get(id); err == nil && inst.Active() {
			if raw := sessionFromInstanceCandidates(inst); raw != "" {
				return raw
			}
		}
	}
	if inst, found, err := bus.ResolveLiveInstanceByLabel(agentName); err == nil && found {
		if raw := sessionFromInstanceCandidates(inst); raw != "" {
			return raw
		}
	}
	if raw := sessionForBindingOrTool(agentName); raw != "" {
		return raw
	}
	return ""
}

func sessionFromInstanceCandidates(inst fleet.Instance) string {
	for _, candidate := range append([]string{inst.Label, inst.Family}, inst.Bindings...) {
		if raw := sessionForBindingOrTool(candidate); raw != "" {
			return raw
		}
	}
	return ""
}

func sessionForBindingOrTool(name string) string {
	toolName, _, _ := strings.Cut(strings.TrimSpace(name), ":")
	if tool, ok := fleet.New().Tool(toolName); ok {
		return tool.CurrentSession()
	}
	if tool, ok := fleet.New().Tool(bus.MainLineBase(name)); ok {
		return tool.CurrentSession()
	}
	return ""
}

func isSteerToolName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	t, ok := fleet.New().Tool(name)
	if !ok {
		return false
	}
	_, steer := t.SteerArgvPrefix("")
	return steer
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
				claimIDs := []string{room.AgentClaimID(canon), canon}
				if id, ok := bus.ExplicitInstanceID(canon); ok {
					claimIDs = append([]string{"instance:" + id}, claimIDs...)
				} else if inst, found, _ := bus.ResolveLiveInstanceByLabel(canon); found {
					claimIDs = append([]string{inst.ClaimID()}, claimIDs...)
				}
				for _, cid := range claimIDs {
					if card, live, ferr := room.Find(cid); ferr == nil && live {
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
						break
					}
				}
			}
			if holderPID > 0 && holderMode != "" {
				hint := fmt.Sprintf("pid %d (%s) owns this seat; release it there or take explicitly", holderPID, holderMode)
				if holderMode == "foreman" {
					hint = fmt.Sprintf("foreman pid %d owns this seat; inspect with 'bashy foreman status' or release with 'bashy foreman pause'", holderPID)
				}
				return fmt.Errorf("%w (%s; %s)", err, holderDetail, hint)
			}
			return fmt.Errorf("%w; establish or refresh ownership in the SAME session (run `bashy inbox --as %s --peek`, `bashy inbox --as %s --watch`, or export BASHY_AGENT=%s when unclaimed)", err, as, as, as)
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
