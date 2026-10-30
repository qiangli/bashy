// atlas renders command registration verb rows from the desk tool schema.
package main

import (
	"fmt"
	"os"
	"strconv"

	kit "desk-kit"
)

func main() {
	if len(os.Args) != 3 {
		os.Exit(2)
	}
	offset, err := strconv.Atoi(os.Args[2])
	if err != nil || offset < 0 {
		os.Exit(2)
	}
	verbs := kit.ToolVerbs(os.Args[1])
	if len(verbs) == 0 {
		os.Exit(2)
	}
	for i, verb := range verbs {
		fmt.Printf("args.positionals.0.enum.%d=%s\n", i+offset, verb)
	}
}
