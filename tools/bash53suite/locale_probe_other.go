//go:build !windows

package main

import (
	"fmt"
	"io"
)

func runLocaleProbe(stdout, stderr io.Writer) error {
	return fmt.Errorf("locale probe is Windows-only")
}
