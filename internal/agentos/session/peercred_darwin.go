// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build darwin

package session

import "golang.org/x/sys/unix"

// peerUIDFromFD reads the peer's credentials from the kernel via LOCAL_PEERCRED
// (the BSD/darwin spelling of SO_PEERCRED).
func peerUIDFromFD(fd uintptr) (int, error) {
	xu, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	if err != nil {
		return -1, err
	}
	return int(xu.Uid), nil
}
