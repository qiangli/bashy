// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Command sh is the pure POSIX shell used by the shell conformance gate. It
// shares the lean cmd/bash import graph but selects strict POSIX behavior when
// invoked as sh. The GNU Bash-compatible cmd/bash binary remains separate.
package main

import (
	"fmt"
	"os"

	"github.com/bashsharp/bashsharp/cli"
	"github.com/qiangli/bash/interp/ownedexec"
)

func main() {
	adoptingOwnedFrame := len(os.Args) == 2 && os.Args[1] == ownedexec.Sentinel
	if err := ownedexec.Adopt(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(126)
	}
	if adoptingOwnedFrame {
		cli.AdoptGoSourceProcessEnvironment()
	}
	cli.Main()
}
