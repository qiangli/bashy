//go:build !windows

package main

import (
	"fmt"
	"io"
)

func runFiletimeProbe(_, _, _ string, _, _ io.Writer) error {
	return fmt.Errorf("filetime probe is only supported on Windows")
}
