// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// procStartTime names a live process's incarnation: its kernel start time
// (kern.proc.pid). A zombie is not live. The slice form is used because it
// reports an absent pid as no entries, where SysctlKinfoProc reports EIO —
// indistinguishable from a failed read.
func procStartTime(pid int) (string, error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	if err != nil {
		return "", err
	}
	if len(kps) == 0 {
		return "", fmt.Errorf("%w: no such process", errProcessGone)
	}
	kp := &kps[0]
	if len(kps) != 1 || int(kp.Proc.P_pid) != pid {
		return "", errors.New("unexpected kern.proc.pid result")
	}
	if kp.Proc.P_stat == 5 { // SZOMB
		return "", fmt.Errorf("%w: zombie", errProcessGone)
	}
	t := kp.Proc.P_starttime
	return fmt.Sprintf("%d.%06d", t.Sec, t.Usec), nil
}

func processGroupIdentities(pgid int) ([]processIdentity, error) {
	processes, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pgid)
	if err != nil {
		return nil, err
	}
	var members []processIdentity
	for _, kp := range processes {
		if int(kp.Eproc.Pgid) != pgid || kp.Proc.P_stat == 5 {
			continue
		}
		t := kp.Proc.P_starttime
		members = append(members, processIdentity{int(kp.Proc.P_pid), fmt.Sprintf("%d.%06d", t.Sec, t.Usec)})
	}
	return members, nil
}
