package main

import "github.com/usememos/memos/internal/version"

// The shaped server build receives an extracted source tree without Git
// metadata. Give that build the calendar version of its pinned base.
func init() {
	version.Version = "26.11.13-dev.d0c6e2a"
	rootCmd.Version = version.Version
}
