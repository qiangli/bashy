// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !unix

package agentos

// probeDisk / probeRAM are unix-only (statfs). On other platforms the advisor's
// disk and memory-byte hints are simply unavailable; ok=false keeps it silent
// for those dimensions while the cwd/network/compute cases still work.
func probeDisk(dir string) (freeBytes uint64, readOnly bool, ok bool) {
	return 0, false, false
}

func probeRAM() (uint64, bool) {
	return 0, false
}
