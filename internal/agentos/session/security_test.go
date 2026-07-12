// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux || darwin

package session

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// A same-uid peer — us connecting to our own session — must be authorized.
// Every legitimate use is this case, so if it broke, the fix would have
// disabled the feature rather than secured it.
func TestAuthorizeSameUIDPeerIsAccepted(t *testing.T) {
	c1, c2 := unixPair(t)
	defer c1.Close()
	defer c2.Close()
	if err := authorizePeer(c1); err != nil {
		t.Fatalf("our own uid was rejected: %v", err)
	}
}

// The authority check must fail closed on a connection whose peer credentials
// cannot be read — a non-unix transport stands in for "kernel would not tell us
// who this is". Better to refuse a command than to run an unidentified one.
func TestAuthorizeFailsClosedOnUnknownPeer(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	if err := authorizePeer(c1); err == nil {
		t.Fatal("a peer with no readable credentials was authorized")
	}
}

// Serve must leave its socket 0600 and its directory 0700 — the filesystem half
// of the control, and the only half that protects the socket on macOS.
func TestServeLocksDownSocketPermissions(t *testing.T) {
	dir := shortDir(t)
	// Simulate the historical bug: a pre-existing, group/other-accessible dir.
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "s.sock")

	if err := secureSocketDir(sock); err != nil {
		t.Fatalf("secureSocketDir: %v", err)
	}
	if perm := modeOf(t, dir); perm&0o077 != 0 {
		t.Errorf("dir left accessible to group/other: %#o", perm)
	}

	ln, err := listenSecure(sock)
	if err != nil {
		t.Fatalf("listenSecure: %v", err)
	}
	defer ln.Close()
	if perm := modeOf(t, sock); perm&0o077 != 0 {
		t.Errorf("socket left accessible to group/other: %#o", perm)
	}
}

// shortDir returns a temp dir under os.TempDir() with a path short enough to fit
// a unix socket's sun_path limit (~104 bytes on macOS); the standard t.TempDir()
// path is too long.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp(os.TempDir(), "bst")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func modeOf(t *testing.T, p string) os.FileMode {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func unixPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	dir := shortDir(t)
	sock := filepath.Join(dir, "p.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := ln.Accept()
		ch <- res{c, err}
	}()
	client, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	return r.c, client
}
