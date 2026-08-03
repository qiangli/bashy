package agentos

import (
	"os"
	"strconv"
	"testing"

	"github.com/qiangli/coreutils/pkg/room"
)

// roomInTempHome points the room store at a disposable HOME so a test never
// touches the operator's live session registry.
func roomInTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	if room.Dir() == "" {
		t.Skip("room store unavailable")
	}
}

// Nothing is registered when the shell is not running under an agent. A card
// filed for an ordinary human shell would put a phantom occupant in the address
// book.
func TestShellSession_NoAgentNoCard(t *testing.T) {
	roomInTempHome(t)
	// Clear every env marker a tool might set. CLAUDECODE is the one this host
	// sets; the detector reads a catalog of them, so the kill switch is the
	// reliable way to assert the negative.
	t.Setenv("BASHY_SHELL_PRESENCE", "off")

	registerShellSession()
	members, err := room.Members()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("no card may be filed with presence off, got %+v", members)
	}
}

// THE INVARIANT THAT MAKES THIS SAFE. A presence card must never occupy the id
// space room.Join claims, or filing one would refuse the operator's own
// `bashy chat` launch of the same binding.
func TestShellSession_PresenceIdNeverCollidesWithAClaim(t *testing.T) {
	roomInTempHome(t)

	// A real session claims its binding, exactly as chat.Start does.
	if err := room.Join(room.Card{ID: "claude-opus5", Tool: "claude", Binding: "claude:opus5", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	// A presence card for the same tool must be a DIFFERENT id, so the claim
	// stands and both occupants are visible.
	presence := room.Card{
		ID:      shellSessionPrefix + "claude:" + strconv.Itoa(os.Getppid()),
		Tool:    "claude",
		Binding: "claude",
		Mode:    shellSessionMode,
		PID:     os.Getppid(),
	}
	if presence.ID == "claude-opus5" {
		t.Fatal("presence id collided with a claimed session id")
	}
	if err := room.Join(presence); err != nil {
		t.Fatalf("a presence card must not be refused by an existing claim: %v", err)
	}

	members, err := room.Members()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("both occupants must be visible, got %d: %+v", len(members), members)
	}
}

// A presence card advertises NO control socket. An advertised socket that does
// not exist is the exact lie this work removes: it would make an unreachable
// session read as pushable.
func TestShellSession_PresenceCardAdvertisesNoSocket(t *testing.T) {
	c := room.Card{ID: shellSessionPrefix + "claude:123", Mode: shellSessionMode}
	if c.CtlSock != "" {
		t.Fatal("a presence card must never advertise a control socket")
	}
	if !IsShellPresenceCard(c) {
		t.Fatal("a presence card must be identifiable, or reach cannot be rendered honestly")
	}
	if IsShellPresenceCard(room.Card{ID: "claude-opus5"}) {
		t.Fatal("a claimed session must not be mistaken for a presence card")
	}
}

// A session already registered by a claiming path must not get a second,
// presence-shaped card: one occupant reported twice reads as a collision that
// does not exist.
func TestShellSession_SkipsWhenThePidIsAlreadyRegistered(t *testing.T) {
	roomInTempHome(t)
	ppid := os.Getppid()

	// A claiming path filed a card at the very pid registerShellSession would use.
	if err := room.Join(room.Card{ID: "claude-opus5", Tool: "claude", Binding: "claude:opus5", PID: ppid}); err != nil {
		t.Fatal(err)
	}
	registerShellSession()

	members, err := room.Members()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range members {
		if IsShellPresenceCard(m) {
			t.Fatalf("a pid already registered must not gain a presence card: %+v", m)
		}
	}
}
