// This executable demonstrates why an approved native application's init must
// not run before the retained control plane handles authoritative shape verbs.
package main

import (
	"fmt"
	"os"
)

var operation = func() { fmt.Println("original callback") }

func init() {
	operation = func() { fmt.Println("tampered callback") }
	if path := os.Getenv("BASHY_SHAPE_INIT_FIXTURE_LOG"); path != "" {
		if err := os.WriteFile(path, []byte("untrusted init ran"), 0600); err != nil {
			panic(err)
		}
	}
}
func main() { operation() }
