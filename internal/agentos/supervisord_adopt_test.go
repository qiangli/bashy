// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/service"
)

// startFakeOrphan runs the unit's own command with no ownership record, as
// an older monitor's leftover would be, and waits until it holds the port.
func startFakeOrphan(t *testing.T, u fleet.Service, port, home string) *exec.Cmd {
	t.Helper()
	c := exec.Command(u.Exec[0], u.Exec[1:]...)
	c.Env = append(os.Environ(), "BASHY_HOME="+home, "BASHY_SUPERVISORD_TEST_CHILD=listen:"+port)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _ = c.Wait() })
	deadline := time.Now().Add(20 * time.Second)
	for checkPorts(u) == nil {
		if time.Now().After(deadline) {
			t.Fatal("orphan never took the port")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return c
}

// The 2026-11-10 outage: an orphan of the unit's own command, from an earlier
// supervisor that left no record, holds the unit's port. The monitor replaces
// it and starts the unit instead of failing it as a foreign listener.
func TestMonitorReplacesOwnRecordlessOrphanHoldingUnitPort(t *testing.T) {
	needStartTime(t)
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	port := freePort(t)
	u := listenerUnit(port)
	orphan := startFakeOrphan(t, u, port, home)

	set := &unitSet{units: map[string]fleet.Service{u.Name: u}}
	m := newMonitor()
	m.load = set.load
	m.providers = service.Providers{fleet.ServiceProviderProcess: newProcessProvider(os.Args[0], t.TempDir(), nil)}
	m.resolve = resolveUnitRuntime
	m.replaceOrphans = func(u fleet.Service) []string { return replaceLookalikes(u, os.Args[0], os.Getenv("BASHY_HOME")) }
	sigs, done := make(chan os.Signal, 1), make(chan int, 1)
	go func() { done <- m.run(sigs) }()
	st := waitState(t, m, u.Name, service.Active)
	if st.PID == 0 || st.PID == orphan.Process.Pid {
		t.Fatalf("unit did not start its own incarnation: %+v", st)
	}
	sigs <- os.Interrupt
	<-done
}

// Not provably the unit's own — a different BASHY_HOME, or a different
// program — stays foreign and is never signaled.
func TestReplaceLookalikesSparesForeignProcess(t *testing.T) {
	needStartTime(t)
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	port := freePort(t)
	u := listenerUnit(port)
	orphan := startFakeOrphan(t, u, port, t.TempDir()) // another home
	if lines := replaceLookalikes(u, os.Args[0], home); len(lines) != 0 {
		t.Fatalf("replaced a process of another home: %q", lines)
	}
	other := u
	other.Exec = []string{os.Args[0], "something-else"}
	if lines := replaceLookalikes(other, os.Args[0], os.Getenv("BASHY_HOME")); len(lines) != 0 {
		t.Fatalf("replaced a process with other argv: %q", lines)
	}
	if err := checkPorts(u); !errors.Is(err, errForeignListener) || !sameProcessPID(orphan.Process.Pid) {
		t.Fatalf("foreign listener should stay: %v", err)
	}
}

func sameProcessPID(pid int) bool { _, err := procStartTime(pid); return err == nil }
