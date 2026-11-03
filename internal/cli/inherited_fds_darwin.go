//go:build darwin

package cli

import (
	"os"
	"slices"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// Native children may receive process-substitution descriptors above 255.
// Enumerate the descriptor directory instead of imposing a second, smaller
// limit than the process's actual open-file limit.
func discoverInheritedFds() []int {
	entries, err := os.ReadDir("/dev/fd")
	var fds []int
	if err == nil {
		for _, entry := range entries {
			fd, err := strconv.Atoi(entry.Name())
			if err != nil || fd < 3 {
				continue
			}
			// ReadDir's own transient descriptor has already been closed.
			if _, err := unix.FcntlInt(uintptr(fd), syscall.F_GETFD, 0); err == nil {
				fds = append(fds, fd)
			}
		}
		slices.Sort(fds)
		return fds
	}
	// Preserve the existing fallback if this host has no descriptor directory.
	for fd := 3; fd < 256; fd++ {
		if _, err := unix.FcntlInt(uintptr(fd), syscall.F_GETFD, 0); err == nil {
			fds = append(fds, fd)
		}
	}
	return fds
}
