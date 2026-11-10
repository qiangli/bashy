//go:build unix

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
)

// Replacing a unit's own look-alike orphan. The ownership record
// (supervisord_owner.go) covers a child of a monitor that left a record; it
// cannot cover one left by an older monitor or a record that was lost, which
// then holds the unit's port and fails the unit as a foreign listener (the
// 2026-11-10 dev-box outage: a 2-day-old `bashy llm serve`, ppid 1).
//
// Such a process is replaced, never trusted, and only when it is provably this
// unit's: it listens on a declared port, its argv is the unit's own exec
// (same program name, same arguments) and its environment names the same
// BASHY_HOME as this monitor. Anything else on the port stays foreign.

// Seams, replaced in tests.
var (
	portListeners = lsofListeners
	procCommand   = psCommand
)

// lsofListeners returns the pids listening on tcp port.
func lsofListeners(port int) []int {
	out, err := exec.Command(lsofPath(), "-nP", "-t", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if n, err := strconv.Atoi(f); err == nil && n > 0 {
			pids = append(pids, n)
		}
	}
	return pids
}

// lsofPath finds lsof even under a launchd PATH that lacks /usr/sbin.
func lsofPath() string {
	if p, err := exec.LookPath("lsof"); err == nil {
		return p
	}
	return "/usr/sbin/lsof"
}

// psCommand returns pid's argv followed by its environment as one string
// (`ps -E` / `ps e`), which is enough to match a program, its arguments and one
// BASHY_HOME=value pair.
func psCommand(pid int) string {
	out, err := exec.Command("/bin/ps", "-ww", envFlag(), "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// envFlag is how ps prints the environment after the command: BSD ps (macOS)
// spells it -E, procps (Linux) the BSD-style word e.
func envFlag() string {
	if runtime.GOOS == "darwin" {
		return "-E"
	}
	return "e"
}

// ownLookalike reports whether command (argv then environment) is a run of
// the unit's exec under bashyHome. self is what a leading `bashy` resolves to.
func ownLookalike(command string, argv []string, self, bashyHome string) bool {
	if command == "" || len(argv) == 0 {
		return false
	}
	prog := argv[0]
	if prog == "bashy" && self != "" {
		prog = self
	}
	base := func(s string) string { return strings.TrimSuffix(filepath.Base(s), ".real") }
	fields := strings.Fields(command)
	if len(fields) < len(argv) || base(fields[0]) != base(prog) {
		return false
	}
	for i, a := range argv[1:] {
		if fields[i+1] != a {
			return false
		}
	}
	rest := fields[len(argv):]
	home := ""
	for _, f := range rest {
		if v, ok := strings.CutPrefix(f, "BASHY_HOME="); ok {
			home = v
		}
	}
	return effectiveBashyHome(home) == effectiveBashyHome(bashyHome)
}

// effectiveBashyHome resolves an unset BASHY_HOME to the default ~/.bashy.
func effectiveBashyHome(v string) string {
	if v = strings.TrimSpace(v); v == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			h = os.TempDir()
		}
		v = filepath.Join(h, ".bashy")
	}
	return filepath.Clean(v)
}

// replaceLookalikes stops every process that listens on u's declared tcp
// ports and is provably u's own orphan, returning one line per process.
func replaceLookalikes(u fleet.Service, self, bashyHome string) []string {
	argv, err := u.ExpandExec("")
	if err != nil || len(argv) == 0 {
		return nil
	}
	var out []string
	seen := map[int]bool{}
	for _, p := range u.Ports {
		if p.Protocol != "" && p.Protocol != "tcp" {
			continue
		}
		for _, pid := range portListeners(p.Port) {
			if seen[pid] || pid == os.Getpid() {
				continue
			}
			seen[pid] = true
			if !ownLookalike(procCommand(pid), argv, self, bashyHome) {
				continue
			}
			if stopPID(pid, supervisordGrace) {
				out = append(out, fmt.Sprintf("%s: replaced orphan pid %d holding port %d", u.Name, pid, p.Port))
			} else {
				out = append(out, fmt.Sprintf("%s: orphan pid %d holding port %d survived SIGKILL", u.Name, pid, p.Port))
			}
		}
	}
	return out
}

// stopPID TERMs pid (and its group when it leads one), then KILLs after grace.
func stopPID(pid int, grace time.Duration) bool {
	signal := func(s syscall.Signal) {
		if pgid, err := syscall.Getpgid(pid); err == nil && pgid == pid {
			_ = syscall.Kill(-pid, s)
		}
		_ = syscall.Kill(pid, s)
	}
	gone := func(d time.Duration) bool {
		deadline := time.Now().Add(d)
		for {
			if err := syscall.Kill(pid, 0); err != nil {
				return true
			}
			if _, err := procStartTime(pid); err != nil { // zombie
				return true
			}
			if time.Now().After(deadline) {
				return false
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	signal(syscall.SIGTERM)
	if gone(grace) {
		return true
	}
	signal(syscall.SIGKILL)
	return gone(5 * time.Second)
}
