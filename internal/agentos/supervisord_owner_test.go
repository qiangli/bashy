// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/qiangli/yoke/pkg/service"
)

func needStartTime(t *testing.T) {
	t.Helper()
	if _, err := procStartTime(os.Getpid()); errors.Is(err, errStartTimeUnsupported) {
		t.Skip(err)
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

func startHeld(t *testing.T) *exec.Cmd {
	t.Helper()
	c := exec.Command(os.Args[0], "held")
	c.Env = append(os.Environ(), "BASHY_SUPERVISORD_TEST_CHILD=hold")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _ = c.Wait() })
	return c
}

func writeOwner(t *testing.T, root, name string, rec unitOwner) string {
	t.Helper()
	path := filepath.Join(root, name, unitOwnerName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(rec)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Ownership is proven, never inferred: a record whose pid now has another
// start time (reused) or whose recording monitor is still alive leaves the
// process alone.
func TestReclaimOrphansSparesUnprovenProcesses(t *testing.T) {
	needStartTime(t)
	root := t.TempDir()
	c := startHeld(t)
	pid := c.Process.Pid
	start, err := procStartTime(pid)
	if err != nil {
		t.Fatal(err)
	}
	reused := writeOwner(t, root, "reused", unitOwner{PID: pid, Start: start + "x", Monitor: 1 << 30, MonitorStart: "gone"})
	// Its own monitor (here: the process itself) is alive — not ours.
	livemon := writeOwner(t, root, "livemon", unitOwner{PID: pid, Start: start, Monitor: pid, MonitorStart: start})
	if lines := reclaimOrphans(root); len(lines) != 0 {
		t.Fatalf("reclaimed something: %q", lines)
	}
	if !sameProcess(pid, start) {
		t.Fatal("an unproven process was signaled")
	}
	if _, err := os.Stat(reused); !os.IsNotExist(err) {
		t.Fatal("a stale record (reused pid) should be dropped")
	}
	if _, err := os.Stat(livemon); err != nil {
		t.Fatal("a live monitor's record must be kept")
	}
}

// The process provider records a running unit and drops the record once the
// child is waited for, so a clean stop leaves nothing to reclaim.
func TestProcessProviderOwnershipRecordLifecycle(t *testing.T) {
	needStartTime(t)
	root := t.TempDir()
	t.Setenv("BASHY_SUPERVISORD_TEST_CHILD", "hold")
	p := newProcessProvider(os.Args[0], root, nil)
	u := testUnit("rec", true)
	u.Exec = []string{os.Args[0], "rec"}
	if err := p.Activate(t.Context(), u, service.Runtime{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "rec", unitOwnerName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rec unitOwner
	_ = json.Unmarshal(b, &rec)
	if rec.PID != p.Status("rec").PID || rec.Monitor != os.Getpid() || rec.Start == "" {
		t.Fatalf("record %+v", rec)
	}
	if err := p.Deactivate(t.Context(), "rec"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("record survived a clean stop")
	}
}
