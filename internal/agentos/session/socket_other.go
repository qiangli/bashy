// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !linux && !darwin

package session

import (
	"fmt"
	"net"
)

// secureSocketDir refuses to host a warm session on a platform where neither of
// the controls it depends on exists.
//
// Windows has AF_UNIX, but its unix sockets do not honor POSIX file modes and
// it offers no peer-credential option — so we can neither keep other users out
// of the socket nor tell who is on the far end of it. Since the session runs
// arbitrary shell commands for its caller, serving it under those conditions
// would be handing out code execution.
//
// This costs nothing but speed: `bashy serve` is purely a latency optimization,
// and Route() falls through to normal in-process execution whenever no session
// answers.
func secureSocketDir(string) error {
	return fmt.Errorf("bashy serve is not supported on this platform: " +
		"a warm session cannot be secured here (no socket permissions, no peer-credential check)")
}

func listenSecure(string) (net.Listener, error) {
	return nil, fmt.Errorf("bashy serve is not supported on this platform")
}
