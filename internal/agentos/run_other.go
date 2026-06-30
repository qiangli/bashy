// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !unix

package agentos

import "os"

// procStatus on non-unix uses the portable ExitCode (-1 ⇒ killed; reported as
// signaled with the conventional 137 so the compute hint can still fire).
func procStatus(ps *os.ProcessState) (int, bool) {
	if code := ps.ExitCode(); code >= 0 {
		return code, false
	}
	return 137, true
}
