// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"
	"strconv"

	"golang.org/x/sys/windows"
)

// procStartTime names a live process's incarnation: its creation time. An
// exited process whose handle is still held somewhere is not live.
func procStartTime(pid int) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return "", err
	}
	if code != 259 { // STILL_ACTIVE
		return "", errors.New("exited")
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return "", err
	}
	return strconv.FormatInt(created.Nanoseconds(), 10), nil
}
