//go:build windows

package agentos

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func inboxAncestrySupported() bool { return true }

// inboxParentProcessID reads the parent from a Toolhelp process snapshot. The
// recorded parent may have exited and its PID been reused; the bounded depth
// walk in inboxOwnerRelation tolerates that the same way it does on unix.
func inboxParentProcessID(pid int) (int, bool) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return 0, false
	}
	for {
		if int(pe.ProcessID) == pid {
			return int(pe.ParentProcessID), true
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			return 0, false
		}
	}
}
