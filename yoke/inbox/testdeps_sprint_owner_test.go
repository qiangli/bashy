package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/foreman"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/weave"
)

var (
	runSprintOwnerForeman  = launchSprintOwnerForeman
	loadSprintOwnerState   = func(id string) (foreman.State, error) { return foreman.NewStore("", id).LoadState() }
	sendSprintOwnerTell    = func(id, brief string) (foreman.Ack, error) { return foreman.Tell("", id, brief) }
	stopSprintOwnerForeman = func(id string) error {
		_, err := foreman.SendCommand("", id, foreman.Command{Verb: foreman.CommandStop})
		// A manager that accepts but never answers cannot be stopped over its own
		// socket (Sprint 457); end it only if foreman proves the process is its own.
		if ne, ok := errors.AsType[net.Error](err); ok && ne.Timeout() {
			if terr := foreman.Terminate("", id, "control socket unresponsive"); terr != nil {
				return fmt.Errorf("%w; %w", err, terr)
			}
			return nil
		}
		return err
	}
	runSprintOwnerCommand = func(ctx context.Context, path string, args []string, dir string, env []string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Dir = dir
		cmd.Env = env
		return cmd.CombinedOutput()
	}
	waitSprintOwnerControl      = waitForSprintOwnerControl
	waitSprintOwnerTransport    = waitForSprintOwnerTransport
	sprintOwnerControlSupported = foreman.ControlSupported
)

func launchSprintOwnerForeman(ctx context.Context, id string, req weave.SprintOwnerRequest) error {
	args := sprintOwnerForemanArgs(id, req)
	// The detached manager inherits this environment. Keep it out of the
	// Foreman arguments: Foreman's persisted state has no environment field,
	// so the lease token remains only in the manager process environment and
	// the lease-token file used to create this request.
	env := sprintOwnerLaunchEnv(os.Environ(), req.Env)
	// No human answers a managed manager's prompts: its genie must run the YOLO profile.
	env = append(env, unattendedAgentEnv+"=1")
	out, err := runSprintOwnerCommand(ctx, bashySelfPath(), args, req.Cwd, env)
	if err != nil {
		return fmt.Errorf("start managed session %s: %w: %s", id, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func sprintOwnerLaunchEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	env := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if _, replace := overrides[name]; !replace {
			env = append(env, entry)
		}
	}
	for name, value := range overrides {
		env = append(env, name+"="+value)
	}
	return env
}

func sprintOwnerForemanArgs(id string, req weave.SprintOwnerRequest) []string {
	return []string{"foreman", "start", "--detach", "--id", id,
		"--goal", fmt.Sprintf("Manage sprint #%d", req.Sprint), "--agent", req.Owner,
		"--no-max-runtime", "--opening-send-once", "--yolo"}
}

func waitForSprintOwnerControl(ctx context.Context, id string, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	path := foreman.NewStore("", id).CtlSockPath()
	for {
		// The pathname is not readiness: a stale socket can outlive its listener,
		// and a freshly detached process can publish the path just before Accept
		// is reachable. Prove that the control listener accepts connections before
		// allowing the caller to deliver its send-once instruction.
		if conn, err := net.DialTimeout("unix", path, 100*time.Millisecond); err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("managed session %s did not open its control socket", id)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func waitForSprintOwnerTransport(ctx context.Context, id, owner string, limit time.Duration) (room.OwnerTransport, error) {
	deadline := time.Now().Add(limit)
	for {
		st, stateErr := loadSprintOwnerState(id)
		if stateErr == nil && (st.Blocker != "" || st.Stopped) {
			why := st.Blocker
			if why == "" {
				why = st.StopReason
			}
			return room.TransportNone, fmt.Errorf("managed session %s failed before becoming reachable: %s", id, why)
		}
		if card, live, findErr := room.Find(room.AgentClaimID(owner)); findErr == nil && live {
			if room.HasCapability(card, room.CapInboxDelivery) && card.Mode == "foreman" && card.Task == id && stateErr == nil && st.Steering {
				return room.TransportManaged, nil
			}
			if card.Mode != "foreman" || card.Task != id || !room.HasCapability(card, room.CapInboxDelivery) {
				return room.TransportNone, fmt.Errorf("managed session %s cannot claim %s: live session %s is %s for %s", id, owner, card.ID, card.Mode, card.Task)
			}
		}
		if time.Now().After(deadline) {
			_, why := room.OwnerTransportFor(owner)
			return room.TransportNone, fmt.Errorf("managed session %s did not become reachable: %s", id, why)
		}
		select {
		case <-ctx.Done():
			return room.TransportNone, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
