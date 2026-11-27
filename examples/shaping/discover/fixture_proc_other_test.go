// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !unix

package discover

import "os/exec"

// The fixture tests skip off unix (bashyBin), so there is no group to manage.
func fixtureOwnGroup(*exec.Cmd)  {}
func fixtureKillGroup(*exec.Cmd) {}
