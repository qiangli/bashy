// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import (
	"os/signal"
	"syscall"
)

func signalIgnoreTerm() { signal.Ignore(syscall.SIGTERM, syscall.SIGINT) }
