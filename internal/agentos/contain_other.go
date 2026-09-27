//go:build !linux && !darwin

package agentos

import (
	"fmt"
	"runtime"
)

// No native network isolation here (Windows): @contain runs its children in
// bashy's own image instead (contain_container.go).
func nativeContainSupported() error {
	return fmt.Errorf("no native network isolation on %s", runtime.GOOS)
}

func runNativeContained([]string) int { return containUnsupportedStatus }
