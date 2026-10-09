// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// bash53locales ensures the provisioned Bash 5.3 corpus locale store exists
// and prints its path: set LOCPATH to the printed directory so coreutils'
// locale(1) advertises the corpus set (glibc localedata compiled with
// localedef into the Go locale store). Idempotent; a no-op when current.
//
// Usage: go run ./tools/bash53locales
package main

import (
	"fmt"
	"os"

	"github.com/qiangli/bashy/internal/corpuslocales"
)

func main() {
	store, err := corpuslocales.EnsureCorpusLocales()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bash53locales:", err)
		os.Exit(1)
	}
	fmt.Println(store)
}
