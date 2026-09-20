// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The stash closes the reaper-vs-Wait race: what the reaper took for the main
// child is handed back; what it took during the spawn window for anyone else
// is an orphan.
func TestReaperStashHandsBackTheMainChildOnly(t *testing.T) {
	r := &reaper{stash: map[int]childExit{}}
	r.arm()
	r.stash[41] = childExit{code: 7} // reaped in the spawn window: an orphan
	r.stash[42] = childExit{code: 3}
	r.watch(42)
	if _, ok := r.take(41); ok || r.orphansReaped() != 1 {
		t.Fatalf("pid 41 should have been counted as an orphan (stash=%v orphans=%d)", r.stash, r.orphansReaped())
	}
	ex, ok := r.take(42)
	if !ok || ex.code != 3 {
		t.Fatalf("main child status lost: %+v %v", ex, ok)
	}
	if _, ok := r.take(42); ok {
		t.Fatal("take is not idempotent")
	}
}

// As a subreaper the supervisor adopts the grandchild the root leaves behind,
// reaps it, and still reports the root's own status correctly.
func TestSupervisordReapsOrphansAsSubreaper(t *testing.T) {
	dag := dagFixture(t)
	pidFile := filepath.Join(t.TempDir(), "gc.pid")
	s, out, log := realSupervisor(t, "orphan:"+pidFile, dag)
	r := startReaper()
	if r == nil {
		t.Skip("cannot become a child subreaper here")
	}
	defer r.stop()
	s.reaper = r
	if code := s.run(); code != 0 {
		t.Fatalf("exit = %d\n%s\n%s", code, log, readAll(t, out))
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	gc, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	// The root is gone (exit 0 above), so the grandchild's stdin hit EOF and it
	// is exiting; it was reparented to us and must be reaped, not left a zombie.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if r.orphansReaped() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if r.orphansReaped() < 1 {
		t.Fatalf("grandchild %d was never reaped\n%s", gc, log)
	}
	if err := syscall.Kill(gc, 0); err != syscall.ESRCH {
		st, _ := os.ReadFile("/proc/" + strconv.Itoa(gc) + "/stat")
		t.Fatalf("grandchild %d still exists after reaping (kill 0 = %v): %s", gc, err, st)
	}
	if !strings.Contains(log.String(), "completed (exit 0)") {
		t.Fatalf("the root's own status was lost to the reaper:\n%s", log)
	}
}
