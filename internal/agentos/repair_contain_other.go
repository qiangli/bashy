// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !unix && !windows

package agentos

import "os/exec"

type repairContain struct{}

func (c *repairContain) prepare(*exec.Cmd)      {}
func (c *repairContain) attach(*exec.Cmd) error { return nil }
func (c *repairContain) finish()                {}
