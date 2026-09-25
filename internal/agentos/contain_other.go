//go:build !linux && !darwin

package agentos

import (
	"fmt"
	"runtime"
)

func containSupported() error {
	return fmt.Errorf("network isolation is not supported on %s yet", runtime.GOOS)
}

func runContained([]string) int { return containUnsupportedStatus }
