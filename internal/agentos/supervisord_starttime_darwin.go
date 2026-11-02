// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// procStartTime names a live process's incarnation: its kernel start time
// (kern.proc.pid). A zombie is not live.
func procStartTime(pid int) (string, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return "", err
	}
	if int(kp.Proc.P_pid) != pid {
		return "", errors.New("no such process")
	}
	if kp.Proc.P_stat == 5 { // SZOMB
		return "", errors.New("zombie")
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
