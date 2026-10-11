// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/service"
)

// The stash closes the reaper-vs-Wait race: what the reaper took for the main
// child is handed back; what it took during the spawn window for anyone else
// is an orphan.
func TestReaperStashHandsBackTheMainChildOnly(t *testing.T) {
	r := &reaper{mains: map[int]bool{}, stash: map[int]childExit{}}
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

// The service monitor as a subreaper (what --pid1 asks for; PID 1 is the same
// contract): a unit that leaves an orphan behind has the orphan reaped while
// the unit's own exit status still reaches the restart policy — a completed
// run (exit 0 under on-failure) is not restarted.
func TestServiceMonitorReapsOrphansAsSubreaper(t *testing.T) {
	r := startReaper()
	if r == nil {
		t.Skip("cannot become a child subreaper here")
	}
	defer r.stop()
	pidFile := filepath.Join(t.TempDir(), "gc.pid")
	t.Setenv("BASHY_SUPERVISORD_TEST_CHILD", "orphan:"+pidFile)
	u := fleet.Service{Name: "leaver", Kind: fleet.KindService, Exec: []string{os.Args[0], "leaver"}, Enabled: true}
	set := &unitSet{units: map[string]fleet.Service{u.Name: u}}
	m := newMonitor()
	m.probe = func(ctx context.Context, u fleet.Service, p unitPolicy) (error, bool) { return runProbe(ctx, u, p, r) }
	prov := newProcessProvider(os.Args[0], "", r)
	prov.stdout, prov.stderr = io.Discard, io.Discard
	m.providers = service.Providers{fleet.ServiceProviderProcess: prov}
	m.load = set.load
	m.only, m.force = map[string]bool{u.Name: true}, true
	var log bytes.Buffer
	m.log = &log
	if code := m.run(make(chan os.Signal)); code != 0 {
		t.Fatalf("exit = %d\n%s", code, log.String())
	}
	if !strings.Contains(log.String(), "leaver: starting -> inactive (completed (exit 0))") &&
		!strings.Contains(log.String(), "leaver: active -> inactive (completed (exit 0))") {
		t.Fatalf("the unit's own exit status was lost to the reaper:\n%s", log.String())
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	gc, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && r.orphansReaped() < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	if r.orphansReaped() < 1 {
		t.Fatalf("grandchild %d was never reaped\n%s", gc, log.String())
	}
	if err := syscall.Kill(gc, 0); err != syscall.ESRCH {
		t.Fatalf("grandchild %d still exists after reaping (kill 0 = %v)", gc, err)
	}
}

// Many main children at once (one per unit): a status the reaper collects
// for any live main child is handed back to that child's wait, never counted
// as an orphan.
func TestReaperKeepsEveryLiveMainChild(t *testing.T) {
	r := &reaper{mains: map[int]bool{}, stash: map[int]childExit{}}
	r.arm()
	r.watch(10)
	r.arm()
	r.watch(20)
	r.stash[10] = childExit{code: 1} // as reap() would for a main child
	r.stash[20] = childExit{code: 2}
	r.arm()
	r.stash[30] = childExit{code: 9} // spawn window: unknown yet
	r.disarm()                       // that spawn failed: 30 was an orphan
	if r.orphansReaped() != 1 {
		t.Fatalf("orphans = %d", r.orphansReaped())
	}
	for pid, want := range map[int]int{10: 1, 20: 2} {
		if ex, ok := r.take(pid); !ok || ex.code != want {
			t.Fatalf("main %d: %+v %v", pid, ex, ok)
		}
	}
}
