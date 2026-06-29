// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !linux

package agentos

// probeRAM is currently Linux-only (it reads /proc/meminfo). Elsewhere the
// available-RAM figure is omitted from the compute hint; ok=false. The hint
// itself still fires on the exit-137 OOM signal — only the byte count is
// dropped.
func probeRAM() (uint64, bool) {
	return 0, false
}
