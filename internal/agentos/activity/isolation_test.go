package activity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/coreutils/pkg/room"
)

// TestTransportMustBeStubbedNotRedirected checks both isolation boundaries.
// BASHY_HOME now relocates room state as well as the activity outbox, unless
// BASHY_ROOM_DIR overrides it. The harness still stubs EnsureInbox,
// PublishDurable and WakeLive: these unit tests must observe delivery without
// running the real transport, even when its storage paths are isolated.
func TestTransportMustBeStubbedNotRedirected(t *testing.T) {
	h := newHarness(t)
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_ACTIVITY_DIR", filepath.Join(home, "activity"))
	t.Setenv("BASHY_ROOM_DIR", "")

	roomDir := filepath.Join(home, "room")
	if got := room.Dir(); got != roomDir {
		t.Fatalf("room.Dir() = %s, want %s", got, roomDir)
	}
	if got, want := StateDir(), filepath.Join(home, "activity"); got != want {
		t.Fatalf("StateDir() = %s, want %s", got, want)
	}
	h.live["steward"] = true
	if _, err := Emit(failEvent("isolated transport")); err != nil {
		t.Fatal(err)
	}
	if len(h.inboxes) != 1 || h.inboxes[0] != "steward" || h.publishedTo("steward") != 1 || h.wakeCount("steward") != 1 {
		t.Fatalf("emit did not use every transport stub: inboxes=%v published=%v woken=%v", h.inboxes, h.published, h.woken)
	}
	if _, err := os.Stat(roomDir); !os.IsNotExist(err) {
		t.Fatalf("stubbed emit touched the real room store: stat %s: %v", roomDir, err)
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
