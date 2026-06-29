// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build linux

package agentos

import "os"

// probeRAM returns currently-available memory in bytes, read from
// /proc/meminfo's MemAvailable line (the kernel's own estimate of memory
// usable without swapping). ok=false on any error.
func probeRAM() (uint64, bool) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	return parseMemAvailable(b)
}
