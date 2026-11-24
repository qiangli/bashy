// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !unix

package agentos

import "os/exec"

// mcpHostedContain has no process-group form here: the deadline kills the
// command itself, and the pilot runner's container bounds what it started.
func mcpHostedContain(*exec.Cmd) {}
