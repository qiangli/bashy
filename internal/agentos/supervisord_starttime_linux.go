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
// 22). A zombie is not live — its start time no longer owns anything.
func procStartTime(pid int) (string, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", err
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return "", errors.New("malformed /proc stat")
	}
	f := strings.Fields(s[i+1:]) // f[0] is field 3 (state)
	if len(f) < 20 {
		return "", errors.New("malformed /proc stat")
	}
	if f[0] == "Z" || f[0] == "X" {
		return "", errors.New("zombie")
	}
	boot, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(boot)) + ":" + f[19], nil
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
		b, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		i := strings.LastIndexByte(string(b), ')')
		if i < 0 {
			return nil, errors.New("malformed /proc stat")
		}
		f := strings.Fields(string(b[i+1:]))
		if len(f) < 20 {
			return nil, errors.New("malformed /proc stat")
		}
		if f[2] != strconv.Itoa(pgid) || f[0] == "Z" || f[0] == "X" {
			continue
		}
		members = append(members, processIdentity{pid, strings.TrimSpace(string(boot)) + ":" + f[19]})
	}
	return members, nil
}
