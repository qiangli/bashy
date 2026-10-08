// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"io"

	"github.com/bashsharp/bashsharp/godelta"
)

// dispatchExplain implements `bashy explain <topic>`. The one topic today is
// `go`: the shipped table of Go constructs that differ in Bash#.
func dispatchExplain(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stdout, "usage: bashy explain go [--json|--tsv|--md] [ROW-ID | REFUSAL TEXT...]")
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	if args[0] != "go" {
		fmt.Fprintf(stderr, "bashy explain: unknown topic %q (topics: go)\n", args[0])
		return 2
	}
	return godelta.Main(args[1:], stdout, stderr)
}
