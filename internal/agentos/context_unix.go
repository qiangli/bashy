// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import (
	"bytes"

	"golang.org/x/sys/unix"
)

// unameInfo returns uname -s/-r/-v/-m via the uname(2) syscall (darwin, linux,
// bsd) — no shell-out.
func unameInfo() (sysname, release, version, machine string) {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return "", "", "", ""
	}
	cstr := func(b []byte) string {
		if i := bytes.IndexByte(b, 0); i >= 0 {
			b = b[:i]
		}
		return string(b)
	}
	return cstr(u.Sysname[:]), cstr(u.Release[:]), cstr(u.Version[:]), cstr(u.Machine[:])
}
