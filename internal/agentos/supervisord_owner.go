// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Process ownership across monitor incarnations (S16 rework). A monitor that
// is hard-killed (SIGKILL, OOM) cannot stop its units, so a foreground unit
// survives as an orphan holding its ports, and the monitor's successor would
// refuse those ports as foreign. Each running process unit therefore leaves
// an ownership record next to its log: the child's pid and kernel start time,
// and the same pair for the monitor that started it. At startup — after it
// has won the control socket, so no live monitor is serving — the successor
// stops exactly the recorded process groups whose leader still has the
// recorded identity and whose recording monitor is gone. Ownership is never
// inferred from a port, and a pid whose start time differs (reused) is never
// signaled: a genuinely unrelated listener is still reported as foreign.

const unitOwnerName = "owner.json"

// errStartTimeUnsupported: this OS cannot name a process's start time, so
// ownership cannot be proven and nothing is reclaimed.
var errStartTimeUnsupported = errors.New("process start time unsupported on this OS")

type unitOwner struct {
	PID          int    `json:"pid"`
	Start        string `json:"start"`
	Monitor      int    `json:"monitor_pid"`
	MonitorStart string `json:"monitor_start"`
	Signal       int    `json:"stop_signal"`
	GraceMS      int64  `json:"grace_ms"`
}

// sameProcess reports whether pid is alive (not a zombie) with start time.
func sameProcess(pid int, start string) bool {
	if pid <= 0 || start == "" {
		return false
	}
	got, err := procStartTime(pid)
	return err == nil && got == start
}

// recordOwner writes the ownership record for a just-started unit child. A
// child whose identity cannot be read (unsupported OS, already gone) is left
// unrecorded: it simply cannot be reclaimed.
func recordOwner(root, name string, pid int, pol unitPolicy) string {
	if root == "" {
		return ""
	}
	start, err := procStartTime(pid)
	if err != nil {
		return ""
	}
	mstart, err := procStartTime(os.Getpid())
	if err != nil {
		return ""
	}
	b, _ := json.Marshal(unitOwner{PID: pid, Start: start, Monitor: os.Getpid(), MonitorStart: mstart,
		Signal: int(pol.stopSignal), GraceMS: pol.grace.Milliseconds()})
	path := filepath.Join(unitLogDir(root, name), unitOwnerName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return ""
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) != nil || os.Rename(tmp, path) != nil {
		_ = os.Remove(tmp)
		return ""
	}
	return path
}

// reclaimOrphans stops every unit process a previous, now-dead monitor left
// running under root, and returns one line per record it acted on.
func reclaimOrphans(root string) []string {
	if root == "" {
		return nil
	}
	recs, _ := filepath.Glob(filepath.Join(root, "*", unitOwnerName))
	var out []string
	for _, path := range recs {
		name := filepath.Base(filepath.Dir(path))
		if line := reclaimOne(path, name); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func reclaimOne(path, name string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var rec unitOwner
	if err := json.Unmarshal(b, &rec); err != nil || rec.PID <= 0 {
		_ = os.Remove(path)
		return fmt.Sprintf("%s: dropped unreadable ownership record", name)
	}
	if rec.Monitor != os.Getpid() && sameProcess(rec.Monitor, rec.MonitorStart) {
		// The monitor that started it is still alive (another home sharing
		// this tree): its unit, not ours to touch.
		return ""
	}
	if !sameProcess(rec.PID, rec.Start) {
		_ = os.Remove(path)
		return "" // gone, or the pid now names somebody else
	}
	p, err := os.FindProcess(rec.PID)
	if err != nil {
		return fmt.Sprintf("%s: orphan pid %d: %v", name, rec.PID, err)
	}
	sig := syscall.Signal(rec.Signal)
	if sig == 0 {
		sig = syscall.SIGTERM
	}
	grace := time.Duration(rec.GraceMS) * time.Millisecond
	if grace <= 0 {
		grace = supervisordGrace
	}
	if err := signalProcessGroup(p, sig); err != nil || !waitNotSame(rec.PID, rec.Start, grace) {
		// Still the same process: identity was re-checked just above.
		_ = killProcessGroup(p)
		if !waitNotSame(rec.PID, rec.Start, 5*time.Second) {
			return fmt.Sprintf("%s: orphan pid %d survived SIGKILL", name, rec.PID)
		}
	}
	_ = os.Remove(path)
	return fmt.Sprintf("%s: stopped orphan pid %d left by dead monitor pid %d", name, rec.PID, rec.Monitor)
}

// waitNotSame polls until pid no longer has start (exited, reaped or a
// zombie), or d passes.
func waitNotSame(pid int, start string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if !sameProcess(pid, start) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(25 * time.Millisecond)
	}
}
