package agentos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/pkg/foreman"
	"github.com/qiangli/coreutils/pkg/lockfile"
	"github.com/qiangli/coreutils/pkg/room"
	"github.com/qiangli/coreutils/pkg/weave"
)

const sprintOwnerReadyTimeout = 2 * time.Minute

var sprintOwnerStartMu sync.Mutex

var (
	runSprintOwnerForeman  = launchSprintOwnerForeman
	loadSprintOwnerState   = func(id string) (foreman.State, error) { return foreman.NewStore("", id).LoadState() }
	sendSprintOwnerTell    = func(id, brief string) (foreman.Ack, error) { return foreman.Tell("", id, brief) }
	stopSprintOwnerForeman = func(id string) error {
		_, err := foreman.SendCommand("", id, foreman.Command{Verb: foreman.CommandStop})
		return err
	}
	waitSprintOwnerControl      = waitForSprintOwnerControl
	waitSprintOwnerTransport    = waitForSprintOwnerTransport
	sprintOwnerControlSupported = foreman.ControlSupported
)

func wireSprintOwner() {
	weave.StartSprintOwner = startSprintOwner
	weave.StopSprintOwner = stopSprintOwner
}

func sprintOwnerSessionID(id int64) string { return fmt.Sprintf("sprint-%d-manager", id) }

func startSprintOwner(ctx context.Context, req weave.SprintOwnerRequest) (weave.SprintOwnerSession, error) {
	// Kernel file locks coordinate processes; flock semantics do not serialize
	// two contenders in this same process, so keep the small in-process half too.
	sprintOwnerStartMu.Lock()
	defer sprintOwnerStartMu.Unlock()
	if !sprintOwnerControlSupported() {
		return weave.SprintOwnerSession{}, fmt.Errorf("managed sprint-manager dispatch is not supported on native Windows")
	}
	id := sprintOwnerSessionID(req.Sprint)
	launchLock, err := lockfile.AcquireWithin(foreman.NewStore("", id).Dir()+".launch.lock", sprintOwnerReadyTimeout, lockfile.Holder{
		Name: "sprint-owner-" + id, Intent: "launch or reuse managed sprint manager",
	})
	if err != nil {
		return weave.SprintOwnerSession{}, fmt.Errorf("coordinate managed session %s: %w", id, err)
	}
	defer launchLock.Release()
	reused := false
	if st, err := loadSprintOwnerState(id); err == nil && !st.Stopped {
		if err := waitSprintOwnerControl(ctx, id, 250*time.Millisecond); err == nil {
			if !strings.EqualFold(strings.TrimSpace(st.Agent), strings.TrimSpace(req.Owner)) {
				return weave.SprintOwnerSession{}, fmt.Errorf("managed session %s already belongs to %s", id, st.Agent)
			}
			reused = true
		}
	}

	if !reused {
		if err := runSprintOwnerForeman(ctx, id, req); err != nil {
			return weave.SprintOwnerSession{}, err
		}
		if err := waitSprintOwnerControl(ctx, id, 5*time.Second); err != nil {
			_ = stopSprintOwnerForeman(id)
			return weave.SprintOwnerSession{}, err
		}
	}

	// Send once. Retrying after an ambiguous transport error could duplicate the
	// user's instruction after the daemon had already accepted it.
	ack, err := sendSprintOwnerTell(id, req.Brief)
	if err == nil && (!ack.OK || (!ack.Accepted && !ack.Steered)) {
		err = fmt.Errorf("managed session %s did not acknowledge instruction", id)
	}
	if err != nil {
		if !reused {
			_ = stopSprintOwnerForeman(id)
		}
		return weave.SprintOwnerSession{}, fmt.Errorf("deliver instruction to %s: %w", id, err)
	}
	transport, err := waitSprintOwnerTransport(ctx, id, req.Owner, sprintOwnerReadyTimeout)
	if err != nil {
		if !reused {
			_ = stopSprintOwnerForeman(id)
		}
		return weave.SprintOwnerSession{}, err
	}
	return weave.SprintOwnerSession{ID: id, Reused: reused, Transport: transport}, nil
}

func stopSprintOwner(ctx context.Context, req weave.SprintOwnerRequest) error {
	id := sprintOwnerSessionID(req.Sprint)
	st, err := loadSprintOwnerState(id)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if st.Stopped {
		return nil
	}
	if agent := strings.TrimSpace(st.Agent); agent != "" && !strings.EqualFold(agent, strings.TrimSpace(req.Owner)) {
		return fmt.Errorf("managed session %s belongs to %s, not %s", id, agent, req.Owner)
	}
	path := foreman.NewStore("", id).CtlSockPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if sprintOwnerForemanCardLive(req.Owner, id) {
			return fmt.Errorf("managed session %s is still live but its control socket is missing", id)
		}
		return nil
	}
	if err := stopSprintOwnerForeman(id); err != nil {
		// A socket pathname can outlive its listener. Only suppress the control
		// error when no room card proves this exact Foreman session is still live.
		if sprintOwnerForemanCardLive(req.Owner, id) {
			return err
		}
		_ = os.Remove(path)
		return nil
	}
	// chat.Close gives the TUI five seconds to honor /quit before its caller
	// finishes teardown; leave margin for Foreman to persist the stopped state.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if st, err := loadSprintOwnerState(id); err == nil && st.Stopped {
			return nil
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// The socket may disappear just before the session goroutine releases
			// its room claim. Do not report teardown complete while the exact
			// managed agent is still visibly live; keep waiting for either proof.
			if !sprintOwnerForemanCardLive(req.Owner, id) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("managed session %s accepted stop but did not terminate", id)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func sprintOwnerForemanCardLive(owner, id string) bool {
	card, live, err := room.Find(room.AgentClaimID(owner))
	return err == nil && live && card.Mode == "foreman" && card.Task == id
}

func launchSprintOwnerForeman(ctx context.Context, id string, req weave.SprintOwnerRequest) error {
	args := sprintOwnerForemanArgs(id, req)
	cmd := exec.CommandContext(ctx, bashySelfPath(), args...)
	cmd.Dir = req.Cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("start managed session %s: %w: %s", id, err, strings.TrimSpace(string(out)))
	}
	return nil
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
		if _, err := os.Stat(path); err == nil {
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
