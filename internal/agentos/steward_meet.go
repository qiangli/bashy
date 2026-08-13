package agentos

import (
	"context"
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/meet"
)

var selectMeetAgent = selectStewardAgent
var ensureMeetPermanentRoleRoom = meet.EnsurePermanentRoleRoom

func validateMeetRoomSecretary(name string) error {
	if _, ok := fleet.New().Agent(strings.TrimSpace(name)); !ok {
		return fmt.Errorf("meet: secretary %q must name an agent from `bashy agents list`", name)
	}
	return nil
}

// startMeetPermanentRole assigns the room-local steward role to one concrete
// fleet agent. It deliberately does NOT start the detached host-steward
// supervisor: Address invokes the selected agent for each turn, and a room
// role carries neither the host seat nor authority to write its journal.
//
// The former implementation started `steward start --no-seat` and returned as
// soon as its supervisor process was spawned. That was both stronger than the
// room needed and racy: Meet immediately reloaded the room before the child
// could publish a holder, producing "returned without assigning the role".
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
	sel, err := selectMeetAgent(req.Agent, "", band)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.Agent) == "" {
		if err := randomizeStewardSelection(sel); err != nil {
			return err
		}
	}
	_, err = ensureMeetPermanentRoleRoom(req.Room, req.Role, sel.Chosen.Name, meet.CreateOptions{
		Out: meet.OutStore,
	})
	return err
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
	sel, err := selectMeetAgent(req.Agent, "", selectorBand)
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
