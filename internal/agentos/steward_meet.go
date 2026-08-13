package agentos

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/meet"
)

var runMeetStewardSession = startStewardSession
var selectMeetSecretaryAgent = selectStewardAgent

func validateMeetRoomSecretary(name string) error {
	if _, ok := fleet.New().Agent(strings.TrimSpace(name)); !ok {
		return fmt.Errorf("meet: secretary %q must name an agent from `bashy agents list`", name)
	}
	return nil
}

// startMeetPermanentRole is the process-lifecycle half of Meet's lazy role
// activation. The request itself is a human action in the permanent room, but
// it is not an attended transfer of host authority, so this starts a room
// steward with --no-seat. The agent can manage this room; it cannot write the
// host steward journal until the human explicitly grants and transfers that
// stronger seat through `bashy steward start`.
func startMeetPermanentRole(_ context.Context, req meet.PermanentRoleStartRequest) error {
	if !strings.EqualFold(req.Room, "steward") || !strings.EqualFold(req.Role, "steward") {
		return fmt.Errorf("unsupported permanent role %s in room %s", req.Role, req.Room)
	}
	band := req.Band
	if strings.TrimSpace(req.Agent) != "" {
		// A named agent is the complete selector. selectStewardAgent correctly
		// refuses an agent+band combination because it is otherwise ambiguous.
		band = 0
	}
	return runMeetStewardSession(io.Discard, io.Discard, stewardStartOptions{
		AgentName:       req.Agent,
		Band:            band,
		NoSeat:          true,
		RandomSelection: strings.TrimSpace(req.Agent) == "",
	})
}

// activateMeetRoomSecretary resolves the notes-only role to a concrete agent in
// bashy's fleet. It does not keep a billable process idle: Meet records every
// utterance itself and launches this agent when convergence/minutes actually
// require model work.
func activateMeetRoomSecretary(_ context.Context, req meet.RoomSecretaryStartRequest) (string, error) {
	band := req.Band
	if band == 0 {
		band = 2
	}
	selectorBand := band
	if strings.TrimSpace(req.Agent) != "" {
		selectorBand = 0
	}
	sel, err := selectMeetSecretaryAgent(req.Agent, "", selectorBand)
	if err != nil {
		return "", err
	}
	candidates := append([]stewardCandidate{sel.Chosen}, sel.Runners...)
	for _, candidate := range candidates {
		conflict := false
		for _, excluded := range req.Exclude {
			if strings.EqualFold(strings.TrimSpace(excluded), candidate.Name) {
				conflict = true
				break
			}
		}
		if !conflict {
			return candidate.Name, nil
		}
	}
	return "", fmt.Errorf("no eligible L%d+ secretary remains after excluding the room's participants and chair", band)
}
