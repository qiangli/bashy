package activity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/room"
)

// TestTransportMustBeStubbedNotRedirected is a ratchet on the test harness
// itself, and it exists because the obvious way to isolate these tests does not
// work.
//
// BASHY_ACTIVITY_DIR and BASHY_HOME move THIS package's outbox into a temp
// tree. They do NOT move the bus: room.Dir() reads BASHY_ROOM_DIR and
// otherwise resolves ~/.bashy/room directly. So a harness that set only the
// activity env vars and let Emit call the real bus.Publish would append to the
// OPERATOR'S live room timeline and steer their live agent sessions, while
// looking hermetic — a green suite built on data it did not create.
//
// That is why newHarness replaces EnsureInbox/PublishDurable/WakeLive outright.
// If someone later simplifies the harness back to environment variables, this
// test is the thing that says why they cannot.
func TestTransportMustBeStubbedNotRedirected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_ACTIVITY_DIR", filepath.Join(home, "activity"))

	if got := room.Dir(); strings.HasPrefix(got, home) {
		t.Fatalf("room.Dir() now follows BASHY_HOME (%s); the harness comment is stale and can be simplified", got)
	}
	// Our own store, by contrast, IS redirected.
	if got := StateDir(); !strings.HasPrefix(got, home) {
		t.Fatalf("StateDir() = %s, want it under %s", got, home)
	}
}

// TestEmitTouchesNothingOutsideTheStateDirectory proves the containment claim
// directly: with the transport stubbed, a full emit writes only inside the
// configured outbox.
func TestEmitTouchesNothingOutsideTheStateDirectory(t *testing.T) {
	h := newHarness(t)
	h.live["steward"] = true
	if _, err := Emit(failEvent("failed")); err != nil {
		t.Fatal(err)
	}
	dir := StateDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{journalFile: true, lockFile: true, interestsFile: true}
	for _, e := range entries {
		if !want[e.Name()] {
			t.Fatalf("unexpected file %q in the outbox", e.Name())
		}
	}
	if len(entries) == 0 {
		t.Fatalf("the emit wrote nothing at all")
	}
}
