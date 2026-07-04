// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !unix

package agentos

// unameInfo has no uname on non-unix (e.g. windows); collectSystem falls back to
// runtime.GOOS/GOARCH + os.Hostname.
func unameInfo() (sysname, release, version, machine string) {
	return "", "", "", ""
}
