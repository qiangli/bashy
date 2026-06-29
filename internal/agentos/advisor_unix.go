// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import "syscall"

// probeDisk returns free bytes and read-only state for the filesystem backing
// dir, via statfs. ok=false on any error so the advisor stays silent rather
// than guessing.
func probeDisk(dir string) (freeBytes uint64, readOnly bool, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false, false
	}
	free := st.Bavail * uint64(st.Bsize)
	// ST_RDONLY == 1 across the BSD/macOS and Linux flag layouts.
	ro := st.Flags&1 != 0
	return free, ro, true
}

// probeRAM is best-effort: reliable available-memory accounting differs per
// unix and would need cgo or /proc parsing. The compute hint already fires on
// the strong exit-137 signal, so returning unknown here just omits the byte
// count from the message.
func probeRAM() (uint64, bool) {
	return 0, false
}
