// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !linux

package agentos

// Orphan reaping is a Linux contract (PID 1 / PR_SET_CHILD_SUBREAPER). On every
// other OS the supervisor never adopts anyone's orphans, so there is nothing to
// reap and no reaper: the main child is waited on directly. Nil means "off".
func startReaper() *reaper { return nil }

// reaper is the Linux type; this stub keeps the call sites compiling with the
// same nil-safe methods.
type reaper struct{}

func (*reaper) arm()                       {}
func (*reaper) watch(int)                  {}
func (*reaper) take(int) (childExit, bool) { return childExit{}, false }
func (*reaper) stop()                      {}
