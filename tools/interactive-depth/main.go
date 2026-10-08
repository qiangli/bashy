// interactive-depth runs the same asserted probes as TestInteractiveDepthPTY
// on a native Unix PTY or Windows ConPTY, including from a headless SSH session.
package main

import (
	"fmt"
	"github.com/qiangli/bashy/internal/interactiveprobe"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: interactive-depth PATH_TO_BASH")
		os.Exit(2)
	}
	shell, err := filepath.Abs(os.Args[1])
	if err != nil {
		panic(err)
	}
	failed := 0
	for _, p := range interactiveprobe.Cases {
		out, err := interactiveprobe.Run(shell, p)
		if err != nil {
			failed++
			fmt.Printf("FAIL %s: %v\n%s\n", p.Name, err, out)
		} else {
			fmt.Printf("PASS %s\n", p.Name)
		}
	}
	fmt.Printf("%d probes, %d failures\n", len(interactiveprobe.Cases), failed)
	if failed > 0 {
		os.Exit(1)
	}
}
