package agentos

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/foreman"
	"github.com/qiangli/coreutils/pkg/room"
	"github.com/qiangli/coreutils/pkg/weave"
)

func isolateSprintOwnerHostSeams(t *testing.T) {
	t.Helper()
	oldRun, oldLoad, oldTell, oldStop := runSprintOwnerForeman, loadSprintOwnerState, sendSprintOwnerTell, stopSprintOwnerForeman
	oldControl, oldTransport := waitSprintOwnerControl, waitSprintOwnerTransport
	oldSupported := sprintOwnerControlSupported
	t.Cleanup(func() {
		runSprintOwnerForeman, loadSprintOwnerState = oldRun, oldLoad
		sendSprintOwnerTell, stopSprintOwnerForeman = oldTell, oldStop
		waitSprintOwnerControl, waitSprintOwnerTransport = oldControl, oldTransport
		sprintOwnerControlSupported = oldSupported
	})
	sprintOwnerControlSupported = func() bool { return true }
}

func TestStartSprintOwnerLaunchesAndDeliversTheExactInstructionOnce(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	loadSprintOwnerState = func(string) (foreman.State, error) { return foreman.State{}, os.ErrNotExist }
	launched := 0
	runSprintOwnerForeman = func(_ context.Context, id string, req weave.SprintOwnerRequest) error {
		launched++
		if id != "sprint-42-manager" || req.Owner != "manager" {
			t.Fatalf("launch = %q %+v", id, req)
		}
		return nil
	}
	waitSprintOwnerControl = func(context.Context, string, time.Duration) error { return nil }
	var delivered []string
	sendSprintOwnerTell = func(id, brief string) (foreman.Ack, error) {
		delivered = append(delivered, brief)
		return foreman.Ack{OK: true, Accepted: true}, nil
	}
	waitSprintOwnerTransport = func(context.Context, string, string, time.Duration) (room.OwnerTransport, error) {
		return room.TransportManaged, nil
	}
	stopSprintOwnerForeman = func(string) error { return nil }

	exact := "  keep $HOME literal; do not parse  "
	got, err := startSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 42, Owner: "manager", Brief: exact})
	if err != nil {
		t.Fatal(err)
	}
	if launched != 1 || len(delivered) != 1 || delivered[0] != exact {
		t.Fatalf("launches=%d delivered=%#v", launched, delivered)
	}
	if got.ID != "sprint-42-manager" || got.Reused || got.Transport != room.TransportManaged {
		t.Fatalf("result = %+v", got)
	}
}

func TestStartSprintOwnerReusesMatchingManagedSession(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	loadSprintOwnerState = func(string) (foreman.State, error) { return foreman.State{Agent: "Manager"}, nil }
	runSprintOwnerForeman = func(context.Context, string, weave.SprintOwnerRequest) error {
		t.Fatal("reuse launched a duplicate foreman")
		return nil
	}
	waitSprintOwnerControl = func(context.Context, string, time.Duration) error { return nil }
	calls := 0
	sendSprintOwnerTell = func(string, string) (foreman.Ack, error) {
		calls++
		return foreman.Ack{OK: true, Steered: true}, nil
	}
	waitSprintOwnerTransport = func(context.Context, string, string, time.Duration) (room.OwnerTransport, error) {
		return room.TransportManaged, nil
	}

	got, err := startSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 7, Owner: "manager", Brief: "next"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Reused || calls != 1 {
		t.Fatalf("result=%+v tell calls=%d", got, calls)
	}
}

func TestStartSprintOwnerDoesNotRetryAnAmbiguousDeliveryFailure(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	loadSprintOwnerState = func(string) (foreman.State, error) { return foreman.State{}, os.ErrNotExist }
	runSprintOwnerForeman = func(context.Context, string, weave.SprintOwnerRequest) error { return nil }
	waitSprintOwnerControl = func(context.Context, string, time.Duration) error { return nil }
	calls, stopped := 0, 0
	sendSprintOwnerTell = func(string, string) (foreman.Ack, error) {
		calls++
		return foreman.Ack{}, errors.New("ambiguous ack")
	}
	stopSprintOwnerForeman = func(string) error { stopped++; return nil }

	if _, err := startSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 9, Owner: "manager", Brief: "once"}); err == nil {
		t.Fatal("ambiguous delivery reported success")
	}
	if calls != 1 || stopped != 1 {
		t.Fatalf("tell calls=%d stops=%d", calls, stopped)
	}
}

func TestStartSprintOwnerRejectsNegativeAcknowledgement(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	loadSprintOwnerState = func(string) (foreman.State, error) { return foreman.State{}, os.ErrNotExist }
	runSprintOwnerForeman = func(context.Context, string, weave.SprintOwnerRequest) error { return nil }
	waitSprintOwnerControl = func(context.Context, string, time.Duration) error { return nil }
	sendSprintOwnerTell = func(string, string) (foreman.Ack, error) { return foreman.Ack{}, nil }
	stopped := 0
	stopSprintOwnerForeman = func(string) error { stopped++; return nil }
	if _, err := startSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 10, Owner: "manager", Brief: "once"}); err == nil || !strings.Contains(err.Error(), "acknowledge") {
		t.Fatalf("negative acknowledgement = %v", err)
	}
	if stopped != 1 {
		t.Fatalf("cleanup stops = %d, want 1", stopped)
	}
}

func TestStartSprintOwnerCleansUpAfterControlReadinessFailure(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	loadSprintOwnerState = func(string) (foreman.State, error) { return foreman.State{}, os.ErrNotExist }
	runSprintOwnerForeman = func(context.Context, string, weave.SprintOwnerRequest) error { return nil }
	waitSprintOwnerControl = func(context.Context, string, time.Duration) error { return errors.New("no control") }
	stopped := 0
	stopSprintOwnerForeman = func(string) error { stopped++; return nil }
	if _, err := startSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 11, Owner: "manager", Brief: "once"}); err == nil {
		t.Fatal("control readiness failure reported success")
	}
	if stopped != 1 {
		t.Fatalf("cleanup stops = %d, want 1", stopped)
	}
}

func TestStopSprintOwnerTreatsMissingOrStaleControlSocketAsAlreadyStopped(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	t.Setenv("BASHY_FOREMAN_DIR", t.TempDir())
	loadSprintOwnerState = func(string) (foreman.State, error) { return foreman.State{Agent: "manager"}, nil }
	called := 0
	stopSprintOwnerForeman = func(string) error {
		called++
		return errors.New("connection refused")
	}
	if err := stopSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 12, Owner: "manager"}); err != nil {
		t.Fatal(err)
	}
	if called != 0 {
		t.Fatal("missing control socket attempted a stop")
	}
	path := foreman.NewStore("", sprintOwnerSessionID(12)).CtlSockPath()
	if err := os.MkdirAll(foreman.NewStore("", sprintOwnerSessionID(12)).Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := stopSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 12, Owner: "manager"}); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("stale socket stop calls = %d", called)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale socket was not removed: %v", err)
	}
}

func TestStopSprintOwnerRefusesMissingSocketWhileExactManagerCardIsLive(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	t.Setenv("BASHY_FOREMAN_DIR", t.TempDir())
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	const owner = "manager"
	id := sprintOwnerSessionID(14)
	loadSprintOwnerState = func(string) (foreman.State, error) {
		return foreman.State{Agent: owner}, nil
	}
	stopSprintOwnerForeman = func(string) error {
		t.Fatal("missing socket attempted a control command")
		return nil
	}
	card := room.Card{
		ID: room.AgentClaimID(owner), Nick: owner, Mode: "foreman", Task: id,
		PID: os.Getpid(), Caps: []string{room.CapInboxDelivery},
	}
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.Leave(card.ID) })

	err := stopSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 14, Owner: owner})
	if err == nil || !strings.Contains(err.Error(), "still live") || !strings.Contains(err.Error(), "control socket is missing") {
		t.Fatalf("missing socket with live manager card = %v", err)
	}
}

func TestConcurrentSprintOwnerStartsLaunchOneSession(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	launched := false
	launches, tells := 0, 0
	loadSprintOwnerState = func(string) (foreman.State, error) {
		if launched {
			return foreman.State{Agent: "manager"}, nil
		}
		return foreman.State{}, os.ErrNotExist
	}
	runSprintOwnerForeman = func(context.Context, string, weave.SprintOwnerRequest) error {
		launched = true
		launches++
		return nil
	}
	waitSprintOwnerControl = func(context.Context, string, time.Duration) error { return nil }
	sendSprintOwnerTell = func(string, string) (foreman.Ack, error) {
		tells++
		return foreman.Ack{OK: true, Accepted: true}, nil
	}
	waitSprintOwnerTransport = func(context.Context, string, string, time.Duration) (room.OwnerTransport, error) {
		return room.TransportManaged, nil
	}
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := startSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 13, Owner: "manager", Brief: "work"})
			errCh <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
	if launches != 1 || tells != 2 {
		t.Fatalf("launches=%d tells=%d", launches, tells)
	}
}

func TestSprintOwnerLaunchIsExactOnceAndUnbounded(t *testing.T) {
	args := strings.Join(sprintOwnerForemanArgs("sprint-3-manager", weave.SprintOwnerRequest{
		Sprint: 3, Owner: "manager", Duration: 45 * time.Minute,
	}), " ")
	for _, want := range []string{"--no-max-runtime", "--opening-send-once", "--yolo"} {
		if !strings.Contains(args, want) {
			t.Errorf("launch args missing %s: %s", want, args)
		}
	}
	if strings.Contains(args, " --max-runtime ") || strings.Contains(args, "45m") {
		t.Fatalf("sprint cutoff leaked into manager lifetime: %s", args)
	}
}

func TestSprintOwnerLaunchAuthorizesEverySupportedManagedAgentTool(t *testing.T) {
	for _, owner := range []string{
		"claude-opus5",
		"codex-gpt5.6-sol",
		"agy-opus4.6",
		"opencode-kimi-k3",
		"ycode-gpt5.6-sol",
	} {
		t.Run(owner, func(t *testing.T) {
			args := sprintOwnerForemanArgs("sprint-115-manager", weave.SprintOwnerRequest{
				Sprint: 115, Owner: owner,
			})
			joined := strings.Join(args, " ")
			for _, want := range []string{"--agent " + owner, "--yolo", "--opening-send-once", "--no-max-runtime"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("managed owner launch missing %q: %s", want, joined)
				}
			}
		})
	}
}

func TestStartSprintOwnerRefusesUnsupportedControlBeforeLaunch(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	sprintOwnerControlSupported = func() bool { return false }
	runSprintOwnerForeman = func(context.Context, string, weave.SprintOwnerRequest) error {
		t.Fatal("unsupported platform attempted launch")
		return nil
	}
	if _, err := startSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 5, Owner: "manager", Brief: "work"}); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unsupported platform error = %v", err)
	}
}

func TestWaitSprintOwnerTransportRequiresThisForemanSession(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	const owner = "manager"
	card := room.Card{ID: room.AgentClaimID(owner), Nick: owner, Mode: "chat", Task: "other", PID: os.Getpid(), Caps: []string{room.CapInboxDelivery}}
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.Leave(card.ID) })
	if _, err := waitForSprintOwnerTransport(context.Background(), "sprint-8-manager", owner, time.Second); err == nil || !strings.Contains(err.Error(), "live session") {
		t.Fatalf("unrelated managed card accepted: %v", err)
	}
	room.Leave(card.ID)
	card.Mode, card.Task = "foreman", "sprint-8-manager"
	loadSprintOwnerState = func(string) (foreman.State, error) { return foreman.State{Steering: true}, nil }
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	if got, err := waitForSprintOwnerTransport(context.Background(), "sprint-8-manager", owner, time.Second); err != nil || got != room.TransportManaged {
		t.Fatalf("matching manager transport = %s, %v", got, err)
	}
}
