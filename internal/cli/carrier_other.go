// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !unix

package cli

import "mvdan.cc/sh/v3/interp"

// platformJobCarrier reports that this platform has no OS-backed job carrier:
// there is no fork/exec + signal-number wait model to build one on. newRunner
// then keeps bash-mode background jobs on the synthetic g<N> handles and fails
// POSIX/sh mode closed via unsupportedJobCarrier — strict process semantics
// are never silently faked.
func platformJobCarrier() interp.JobCarrier { return nil }

func resetJobCarrierSignals() {}
