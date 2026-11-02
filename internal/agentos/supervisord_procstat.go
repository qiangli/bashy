// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"syscall"
)

// procFS is the part of Linux /proc that process liveness reads. Only Linux
// wires it to the host (supervisord_starttime_linux.go); it is untagged so
// its fault handling is tested on every platform with injected reads.
//
// Liveness has three answers, never two: live, proven gone (an error
// wrapping errProcessGone), or unknown (any other error). Only a vanished
// /proc entry or a completed thread group is proof of exit; a permission,
// I/O or parse failure is unknown and must not read as completion.
type procFS struct {
	readFile func(string) ([]byte, error)
	readDir  func(string) ([]fs.DirEntry, error)
}

// procVanished reports a /proc read failure that proves the entry is gone.
func procVanished(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH)
}

// stat returns /proc stat fields from field 3 (state) on. A vanished entry
// is returned wrapping errProcessGone.
func (p procFS) stat(path string) ([]string, error) {
	b, err := p.readFile(path)
	if procVanished(err) {
		return nil, fmt.Errorf("%w: %v", errProcessGone, err)
	}
	if err != nil {
		return nil, err
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return nil, errors.New("malformed /proc stat")
	}
	f := strings.Fields(s[i+1:]) // f[0] is field 3 (state)
	if len(f) < 20 {
		return nil, errors.New("malformed /proc stat")
	}
	// These fields decide incarnation equality and group membership. Treat
	// malformed identities as unknown, never as a different (gone) process
	// or an unrelated group member that can be silently omitted.
	for _, field := range []struct {
		index int
		name  string
		bits  int
	}{{2, "pgrp", 32}, {19, "starttime", 64}} {
		if _, err := strconv.ParseUint(f[field.index], 10, field.bits); err != nil {
			return nil, fmt.Errorf("malformed /proc stat %s %q", field.name, f[field.index])
		}
	}
	return f, nil
}

func (p procFS) bootID() (string, error) {
	b, err := p.readFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// startTime names pid's incarnation: the boot it belongs to and its start
// time in clock ticks since that boot (field 22). A thread group whose every
// task has released its resources is not live — its start time no longer
// owns anything.
func (p procFS) startTime(pid int) (string, error) {
	f, err := p.stat("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", err
	}
	exited, err := p.threadGroupExited(pid, f)
	if err != nil {
		return "", err
	}
	if exited {
		return "", fmt.Errorf("%w: thread group %d exited", errProcessGone, pid)
	}
	boot, err := p.bootID()
	if err != nil {
		return "", err
	}
	return boot + ":" + f[19], nil
}

// threadGroupExited decides, from the leader's stat snapshot f, whether every
// thread of group pid has released its descriptors. The leader's state alone
// cannot: a multi-threaded process's leader is a zombie as soon as its own
// thread exits, while siblings sharing the descriptor table (listeners
// included) are still running or on their exit path.
//
// The proof is the same snapshot's num_threads (field 20). The kernel
// releases a non-leader thread — and only then decrements the count — after
// its exit_files, and a zombie leader has already run its own. So a zombie
// leader counting at most itself owns nothing. A count of 0 means the leader
// itself is being released, which happens only after every sibling. An
// unparsable count is unknown, never completion.
//
// /proc/PID/task is consulted only to corroborate: it can keep the group
// live, never prove it gone. Its contents may be unavailable after the main
// thread exits (proc_pid_task(5)), so a missing or empty listing decides
// nothing. A task found running despite the count can only belong to a
// recycled PID or an unexpected kernel; the leader is then re-read, and the
// group is live only while it is still the same incarnation.
func (p procFS) threadGroupExited(pid int, f []string) (bool, error) {
	if f[0] != "Z" && f[0] != "X" {
		return false, nil
	}
	n, err := strconv.Atoi(f[17])
	if err != nil || n < 0 {
		return false, fmt.Errorf("malformed /proc stat num_threads %q", f[17])
	}
	if n > 1 {
		return false, nil
	}
	dir := "/proc/" + strconv.Itoa(pid) + "/task"
	tasks, err := p.readDir(dir)
	if procVanished(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	for _, task := range tasks {
		tf, err := p.stat(dir + "/" + task.Name() + "/stat")
		if errors.Is(err, errProcessGone) {
			continue // proven to have finished exiting while listed
		}
		if err != nil {
			return false, err
		}
		if tf[0] == "Z" || tf[0] == "X" {
			continue
		}
		again, err := p.stat("/proc/" + strconv.Itoa(pid) + "/stat")
		if errors.Is(err, errProcessGone) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return again[19] != f[19], nil
	}
	return true, nil
}

// groupIdentities captures the live members of process group pgid. A member
// whose liveness is unknown fails the snapshot rather than being dropped.
func (p procFS) groupIdentities(pgid int) ([]processIdentity, error) {
	entries, err := p.readDir("/proc")
	if err != nil {
		return nil, err
	}
	boot, err := p.bootID()
	if err != nil {
		return nil, err
	}
	var members []processIdentity
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		f, err := p.stat("/proc/" + entry.Name() + "/stat")
		if errors.Is(err, errProcessGone) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if f[2] != strconv.Itoa(pgid) {
			continue
		}
		exited, err := p.threadGroupExited(pid, f)
		if err != nil {
			return nil, err
		}
		if !exited {
			members = append(members, processIdentity{pid, boot + ":" + f[19]})
		}
	}
	return members, nil
}
