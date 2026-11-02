// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// procStartTime names a live process's incarnation: the boot it belongs to
// and its start time in clock ticks since that boot (/proc/PID/stat field
// 22). A process whose every task has exited is not live — its start time no
// longer owns anything.
func procStartTime(pid int) (string, error) {
	f, err := procStatFields("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", err
	}
	if !threadGroupLive(pid, f[0]) {
		return "", errors.New("zombie")
	}
	boot, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(boot)) + ":" + f[19], nil
}

// procStatFields returns /proc stat fields from field 3 (state) on.
func procStatFields(path string) ([]string, error) {
	b, err := os.ReadFile(path)
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
	return f, nil
}

// threadGroupLive reports whether any task of thread group pid has not yet
// exited. /proc/PID/stat names only the group leader's state, and a
// multi-threaded process's leader is a zombie as soon as its own thread
// exits — while sibling threads, which share and still hold the descriptor
// table (listening sockets included), are still on their exit path. A
// non-leader task leaves /proc/PID/task only after it has released its
// files, so the group is gone once no listed task is short of zombie.
func threadGroupLive(pid int, leaderState string) bool {
	if leaderState != "Z" && leaderState != "X" {
		return true
	}
	dir := "/proc/" + strconv.Itoa(pid) + "/task"
	tasks, err := os.ReadDir(dir)
	if err != nil {
		// Unreadable but present: never call a process gone unproven.
		return !os.IsNotExist(err)
	}
	for _, task := range tasks {
		f, err := procStatFields(dir + "/" + task.Name() + "/stat")
		if err != nil {
			continue // that task finished exiting while we listed it
		}
		if f[0] != "Z" && f[0] != "X" {
			return true
		}
	}
	return false
}

func processGroupIdentities(pgid int) ([]processIdentity, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return nil, err
	}
	var members []processIdentity
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		f, err := procStatFields("/proc/" + entry.Name() + "/stat")
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if f[2] != strconv.Itoa(pgid) || !threadGroupLive(pid, f[0]) {
			continue
		}
		members = append(members, processIdentity{pid, strings.TrimSpace(string(boot)) + ":" + f[19]})
	}
	return members, nil
}
