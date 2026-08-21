// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// bash53fixtures ensures the public GNU Bash 5.3 fixture tree is available in
// bashy's verified user cache and linked into a source checkout.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qiangli/bashy/internal/agentos"
)

func main() {
	root := flag.String("root", ".", "bashy source checkout root")
	flag.Parse()
	abs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bash53fixtures:", err)
		os.Exit(1)
	}
	link, err := agentos.EnsureBash53Fixtures(abs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bash53fixtures:", err)
		os.Exit(1)
	}
	fmt.Println(link)
}
