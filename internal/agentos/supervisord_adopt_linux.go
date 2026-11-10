//go:build linux

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// On Linux the listener and its command come from /proc, so a host without
// lsof or procps (a minimal image) can still prove an orphan is the unit's.
func init() {
	portListeners = procListeners
	procCommand = procCmdEnv
}

// procListeners returns the pids holding a LISTEN tcp socket on port: the
// socket inodes come from /proc/net/tcp{,6}, their owners from /proc/*/fd.
// Falls back to lsof when /proc/net is unreadable.
func procListeners(port int) []int {
	inodes := map[string]bool{}
	read := false
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		read = true
		for _, line := range strings.Split(string(data), "\n")[1:] {
			fs := strings.Fields(line)
			if len(fs) < 10 || fs[3] != "0A" { // 0A = LISTEN
				continue
			}
			_, hexPort, ok := strings.Cut(fs[1], ":")
			if p, err := strconv.ParseInt(hexPort, 16, 32); ok && err == nil && int(p) == port && fs[9] != "0" {
				inodes["socket:["+fs[9]+"]"] = true
			}
		}
	}
	if !read {
		return lsofListeners(port)
	}
	if len(inodes) == 0 {
		return nil
	}
	dirs, _ := filepath.Glob("/proc/[0-9]*/fd")
	var pids []int
	for _, d := range dirs {
		pid, err := strconv.Atoi(filepath.Base(filepath.Dir(d)))
		if err != nil {
			continue
		}
		fds, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			if l, err := os.Readlink(filepath.Join(d, fd.Name())); err == nil && inodes[l] {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids
}

// procCmdEnv is psCommand from /proc: argv then environment, space-joined.
func procCmdEnv(pid int) string {
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	cmd, err := os.ReadFile(filepath.Join(dir, "cmdline"))
	if err != nil || len(cmd) == 0 {
		return psCommand(pid)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "environ"))
	join := func(b []byte) string {
		return strings.TrimSpace(string(bytes.ReplaceAll(bytes.TrimRight(b, "\x00"), []byte{0}, []byte{' '})))
	}
	return strings.TrimSpace(join(cmd) + " " + join(env))
}
