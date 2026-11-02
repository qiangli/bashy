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
	"strconv"
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

// A leader's exit does not release a listener held by another group member.
// Escalation must follow proven process identities, never the occupied port.
func TestReclaimOrphansJoinsTermIgnoringGroupListener(t *testing.T) {
	needStartTime(t)
	root, port := t.TempDir(), freePort(t)
	ready := filepath.Join(root, "ready")
	leader := exec.Command(os.Args[0], "group-listener-leader")
	leader.Env = append(os.Environ(), "BASHY_SUPERVISORD_TEST_CHILD=group-listener-leader:"+port+","+ready)
	leader.SysProcAttr = ownProcessGroupAttr()
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-leader.Process.Pid, syscall.SIGKILL); _ = leader.Wait() })
	var listener int
	deadline := time.Now().Add(5 * time.Second)
	for {
		if b, err := os.ReadFile(ready); err == nil {
			listener, _ = strconv.Atoi(string(b))
			if listener > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("listener never ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if group, err := syscall.Getpgid(listener); err != nil || group != leader.Process.Pid {
		t.Fatalf("listener group=%d error=%v", group, err)
	}
	start, err := procStartTime(leader.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	listenerStart, err := procStartTime(listener)
	if err != nil {
		t.Fatal(err)
	}
	writeOwner(t, root, "listener", unitOwner{PID: leader.Process.Pid, Start: start, Monitor: 1 << 30, MonitorStart: "gone", Signal: int(syscall.SIGTERM), GraceMS: 100})
	if err := checkPorts(listenerUnit(port)); !errors.Is(err, errForeignListener) {
		t.Fatalf("precondition listener: %v", err)
	}
	lines := reclaimOrphans(root)
	if len(lines) != 1 || sameProcess(listener, listenerStart) {
		t.Fatalf("reclaim returned before owned listener exited: %q", lines)
	}
	if err := checkPorts(listenerUnit(port)); err != nil {
		t.Fatalf("reclaimed listener still holds port: %v", err)
	}
}

func TestCapturedOrphanEscalationSparesChangedIdentity(t *testing.T) {
	needStartTime(t)
	child := startHeld(t)
	start, err := procStartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	killCapturedProcesses([]processIdentity{{child.Process.Pid, start + "-stale"}})
	if !sameProcess(child.Process.Pid, start) {
		t.Fatal("escalation signaled a different incarnation")
	}
}
