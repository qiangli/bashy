//go:build darwin || linux

package agentos

import (
	"context"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/foreman"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/weave"
)

// hungSprintOwnerServeEnv switches this test binary into a fixture serve
// process for one isolated foreman session: it stamps itself as the serve
// process exactly as foreman's ServeControl does, then holds a control socket
// that accepts and never answers — the Sprint 457 hung manager. Its store,
// room and home are the parent test's temp dirs; no real session is involved.
const hungSprintOwnerServeEnv = "BASHY_TEST_HUNG_SPRINT_OWNER_SERVE"

func TestHungSprintOwnerServeHelper(t *testing.T) {
	id := os.Getenv(hungSprintOwnerServeEnv)
	if id == "" {
		t.Skip("fixture child only")
	}
	store := foreman.NewStore("", id)
	st, err := store.LoadState()
	if err != nil {
		os.Exit(3)
	}
	st.ServePID = os.Getpid()
	st.ServeStart, _ = foreman.ProcessStart(st.ServePID)
	if err := store.SaveState(st); err != nil {
		os.Exit(4)
	}
	ln, err := net.Listen("unix", store.CtlSockPath())
	if err != nil {
		os.Exit(5)
	}
	var held []net.Conn
	for {
		c, err := ln.Accept()
		if err != nil {
			os.Exit(6)
		}
		held = append(held, c) // accepted, never read, never answered
	}
}

func TestStopSprintOwnerTerminatesAProvenOwnedHungManager(t *testing.T) {
	isolateSprintOwnerHostSeams(t)
	foremanDir, err := os.MkdirTemp("", "s467fm") // short: a unix socket path is capped near 104 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(foremanDir) })
	t.Setenv("BASHY_FOREMAN_DIR", foremanDir)
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	const owner = "fixture-manager"
	id := sprintOwnerSessionID(457)
	store := foreman.NewStore("", id)
	if err := store.SaveState(foreman.State{ID: id, Agent: owner, Status: foreman.StatusWorking}); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestHungSprintOwnerServeHelper$")
	cmd.Env = append(os.Environ(), hungSprintOwnerServeEnv+"="+id)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }() // reap at once, so exit reads as dead, not zombie
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-exited
	})
	for deadline := time.Now().Add(10 * time.Second); ; {
		st, _ := store.LoadState()
		if _, err := os.Stat(store.CtlSockPath()); err == nil && st.ServePID == cmd.Process.Pid {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("hung manager fixture never came up")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The live card is what made the real hung manager unstoppable: stopSprintOwner
	// only forgives a failed stop when no card proves the session is still running.
	card := room.Card{ID: room.AgentClaimID(owner), Nick: owner, Mode: "foreman", Task: id, PID: cmd.Process.Pid}
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.Leave(card.ID) })

	if err := stopSprintOwner(context.Background(), weave.SprintOwnerRequest{Sprint: 457, Owner: owner}); err != nil {
		t.Fatalf("stop of a proven-owned hung manager: %v", err)
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("hung manager is still running after a successful stop")
	}
	if st, err := store.LoadState(); err != nil || !st.Stopped {
		t.Fatalf("hung manager not recorded stopped: %+v %v", st, err)
	}
}
