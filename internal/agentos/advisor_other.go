// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !unix

package agentos

// probeDisk is unix-only (statfs); on other platforms the disk hint is simply
// unavailable. ok=false keeps the advisor silent for that dimension while the
// cwd / network / compute cases still work.
func probeDisk(dir string) (freeBytes uint64, readOnly bool, ok bool) {
	return 0, false, false
}
