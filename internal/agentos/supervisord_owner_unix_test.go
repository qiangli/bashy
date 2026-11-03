// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/service"
)

// The S18 hard-kill regression: a monitor is SIGKILLed while its foreground
// unit holds a declared port. Its successor must stop that recorded orphan
// and bring the unit up — not refuse its own predecessor's child as a foreign
// listener — while an unrelated listener on the port stays foreign.
func TestServiceMonitorReclaimsOrphanOfHardKilledMonitor(t *testing.T) {
	needStartTime(t)
	root, port := t.TempDir(), freePort(t)
	mon := exec.Command(os.Args[0], "monitor")
	mon.Env = append(os.Environ(), "BASHY_SUPERVISORD_TEST_CHILD=monitor:"+root+","+port)
	if err := mon.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mon.Process.Kill(); _ = mon.Wait() }()
	path := filepath.Join(root, "listener", unitOwnerName)
	var rec unitOwner
	deadline := time.Now().Add(20 * time.Second)
	for {
		b, err := os.ReadFile(path)
		if err == nil && json.Unmarshal(b, &rec) == nil && rec.PID > 0 {
			if c, derr := net.Dial("tcp", "127.0.0.1:"+port); derr == nil {
				c.Close()
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("first monitor never brought the listener up")
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(func() { _ = syscall.Kill(-rec.PID, syscall.SIGKILL) })

	// Hard-kill the monitor: nothing gets to stop its unit.
	_ = mon.Process.Signal(syscall.SIGKILL)
	_ = mon.Wait()
	if !sameProcess(rec.PID, rec.Start) {
		t.Fatal("precondition: the unit should have survived its monitor")
	}
	u := listenerUnit(port)
	if err := checkPorts(u); !errors.Is(err, errForeignListener) {
		t.Fatalf("precondition: the orphan should hold the port, got %v", err)
	}

	lines := reclaimOrphans(root)
	if len(lines) != 1 || sameProcess(rec.PID, rec.Start) {
		t.Fatalf("orphan %d not reclaimed: %q", rec.PID, lines)
	}

	set := &unitSet{units: map[string]fleet.Service{u.Name: u}}
	m := newMonitor()
	m.load = set.load
	m.providers = service.Providers{fleet.ServiceProviderProcess: newProcessProvider(os.Args[0], root, nil)}
	m.resolve = resolveUnitRuntime
	sigs, done := make(chan os.Signal, 1), make(chan int, 1)
	go func() { done <- m.run(sigs) }()
	st := waitState(t, m, u.Name, service.Active)
	if st.PID == 0 || st.PID == rec.PID {
		t.Fatalf("successor did not start its own incarnation: %+v", st)
	}
	sigs <- syscall.SIGTERM
	<-done

	// A genuinely foreign listener (no record) is still refused with a reason.
	l, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if lines := reclaimOrphans(root); len(lines) != 0 {
		t.Fatalf("reclaimed with no record: %q", lines)
	}
	if err := checkPorts(u); !errors.Is(err, errForeignListener) {
		t.Fatalf("foreign listener not reported: %v", err)
	}
}
