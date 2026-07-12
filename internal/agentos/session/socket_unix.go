// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux || darwin

package session

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// secureSocketDir creates (or validates) the socket's parent directory as
// owner-only, and returns an error if it cannot be trusted.
//
// The directory — not the socket file — is the load-bearing control. On
// macOS/BSD a unix socket's own mode is not consulted when a peer connects, so
// only the traversal permission on the containing directory keeps other users
// out. This is why OpenSSH puts its agent socket in a 0700 directory rather than
// relying on the socket's mode. We do both, and then still check peer
// credentials, because a permission bit approximates an identity, it is not one.
func secureSocketDir(socket string) error {
	dir := filepath.Dir(socket)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("session dir: %w", err)
	}
	// MkdirAll is a no-op on an existing directory, so what is there may be
	// anybody's, with any mode, or not a directory at all. Verify, don't assume.
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("session dir: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("session dir %s is not a real directory", dir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("refusing to serve: session dir %s is owned by uid %d, not %d",
			dir, st.Uid, os.Getuid())
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		// Tighten rather than refuse: an older bashy created this directory 0755,
		// and failing here would strand every existing user on an error they did
		// not cause. If we cannot tighten it, we do refuse.
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("session dir %s is mode %#o and cannot be tightened: %w", dir, perm, err)
		}
	}
	return nil
}

// listenSecure binds the socket with no group/other permission bits.
//
// The umask is set across the bind rather than chmod'ing afterwards: bind-then-
// chmod leaves a window in which the socket exists and is world-accessible, and
// a race that small is still a race. The explicit Chmod after it is belt and
// braces — umask can only clear bits, but an exotic filesystem should not get to
// decide this.
func listenSecure(socket string) (net.Listener, error) {
	old := syscall.Umask(0o077)
	ln, err := net.Listen("unix", socket)
	syscall.Umask(old)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socket, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("secure %s: %w", socket, err)
	}
	return ln, nil
}
