//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "windowswslocale: this provider runs only on Windows")
	os.Exit(2)
}
