// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux || darwin

package session

import (
	"fmt"
	"net"
	"os"
)

// peerUID returns the uid of the process on the other end of conn.
//
// This is the authoritative check. Filesystem permissions on the socket and its
// directory are the first line (and on macOS the socket's own mode is not even
// enforced on connect — only the directory's is), but they answer "who could
// open this path", not "who is actually talking to me". A kernel-supplied peer
// credential cannot be spoofed by the caller.
func peerUID(conn net.Conn) (int, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return -1, fmt.Errorf("not a unix socket")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return -1, err
	}
	var (
		uid     int
		inerr   error
		ctrlErr = raw.Control(func(fd uintptr) {
			uid, inerr = peerUIDFromFD(fd)
		})
	)
	if ctrlErr != nil {
		return -1, ctrlErr
	}
	if inerr != nil {
		return -1, inerr
	}
	return uid, nil
}

// authorizePeer rejects a connection from any uid but our own.
//
// A warm session executes arbitrary shell commands on behalf of whoever
// connects, so an unauthenticated peer is remote code execution as the session
// owner. Same-uid is the only trust boundary that means anything here: a process
// already running as us can do everything the session could do anyway.
func authorizePeer(conn net.Conn) error {
	uid, err := peerUID(conn)
	if err != nil {
		// Fail CLOSED. If we cannot establish who is calling, we do not run
		// their command.
		return fmt.Errorf("cannot identify peer: %w", err)
	}
	if me := os.Getuid(); uid != me {
		return fmt.Errorf("refusing command from uid %d (session belongs to uid %d)", uid, me)
	}
	return nil
}
