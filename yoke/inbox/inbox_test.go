package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/fleet/fleettest"
	"github.com/qiangli/yoke/pkg/meet"
)

func isolateUnifiedInbox(t *testing.T) {
	t.Helper()
	// Root HOME, BASHY_HOME, sprint, foreman, bus, room, meet, supervisor, and
	// stage-announcement defaults inside one isolated temp HOME, with cloudbox
	// mail delivery and detached foreman launches stubbed off.
	home := isolateSprintForemanTest(t)
	// The peer these tests talk to ("claude-opus5") used to ship in the
	// embedded fleet baseline; that ring is tools-only now, so the test ring
	// supplies the binding the same way an operator's overlay would.
	fleettest.Ring(t)
	fleetDir := filepath.Join(home, ".config", "bashy")
	t.Setenv("BASHY_FLEET_DIR", fleetDir)
	t.Setenv("USER", "tester")
	registerTestInboxAgent(t, fleetDir, inboxTestReader)
}

// inboxTestReader is the name these tests read as. It is registered into the
// test's own empty fleet ring rather than borrowed from the shipped baseline.
//
// The tests used to read as a REAL baseline agent, because meet.Create refuses
// an unregistered participant and the baseline was the only thing that
// resolved. That made a live fleet identity -- somebody's actual conductor --
// the subject of every run, and the inbox path writes to whatever sprint lease
// that name holds. Owning the name is the fix: the ring is already isolated,
// so the test can simply put its own agent in it.
const inboxTestReader = "inbox-test-reader"

func registerTestInboxAgent(t *testing.T, dir, name string) {
	t.Helper()
	agents := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "name: " + name + "\nkind: agent\ntool: claude\nmodel: opus5\n"
	if err := os.WriteFile(filepath.Join(agents, name+".yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := fleet.New().Agent(name); !ok {
		t.Fatalf("test agent %q did not register in the isolated fleet ring %s", name, dir)
	}
}

func TestInboxWatchDeliversANewBoardPostWithoutHumanRelay(t *testing.T) {
	isolateUnifiedInbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	posted := make(chan error, 1)
	go func() {
		time.Sleep(30 * time.Millisecond)
		posted <- bus.PostMessage(bus.Post{From: "bob", To: "alice", Body: "arrived while watching"})
	}()
	var out, errOut bytes.Buffer
	if err := runUnifiedInbox(ctx, &out, &errOut, "alice", 0, false, false, false, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "arrived while watching") {
		t.Fatalf("bounded watch did not deliver new input: %q", out.String())
	}
}

func TestInboxBoundedWaitDoesNotFinishOnOwnMeetPost(t *testing.T) {
	isolateUnifiedInbox(t)
	reader, peer := inboxTestReader, "claude-opus5"
	st, err := meet.Create(meet.CreateOptions{Topic: "sprint channel", Board: true, Participants: []string{reader, peer}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	posted := make(chan error, 2)
	go func() {
		time.Sleep(30 * time.Millisecond)
		posted <- meet.AppendEvent(st.ID, meet.Event{Speaker: reader, Kind: "message", Text: "A status"})
		time.Sleep(120 * time.Millisecond)
		posted <- meet.AppendEvent(st.ID, meet.Event{Speaker: peer, To: reader, Kind: "message", Text: "B reply"})
	}()

	var out, errOut bytes.Buffer
	started := time.Now()
	if err := runUnifiedInbox(context.Background(), &out, &errOut, reader, 0, false, false, false, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
		t.Fatalf("bounded wait finished on reader's own post after %s", elapsed)
	}
	if !strings.Contains(out.String(), "B reply") || strings.Contains(out.String(), "A status") {
		t.Fatalf("bounded wait stdout=%q, want only B reply", out.String())
	}
}
