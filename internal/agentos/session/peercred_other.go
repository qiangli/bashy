// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !linux && !darwin

package session

import (
	"fmt"
	"net"
)

// authorizePeer fails closed on platforms where we cannot ask the kernel who is
// on the other end of the socket.
//
// Windows has AF_UNIX but no peer-credential option, and its unix sockets do not
// honor POSIX file modes, so neither of the two controls we rely on elsewhere is
// available. Rather than serve arbitrary commands to an unidentifiable caller,
// the warm session simply does not accept connections there — `bashy serve` is a
// latency optimization, and every call falls back to normal in-process execution
// when no session answers. Losing speed is acceptable; serving RCE is not.
func authorizePeer(net.Conn) error {
	return fmt.Errorf("the warm session is not supported on this platform (no peer-credential check available)")
}
