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

const nativeModelEgress = false

func runNativeContained([]string, bool) int { return containUnsupportedStatus }

// confineBackend: no backend here yet; fail closed rather than degrade.
func confineBackend(string) (string, error) {
	return "", fmt.Errorf("file confinement (--dir) is not available on %s: no Seatbelt or namespace backend, and bashy's image route is not wired for it here", runtime.GOOS)
}

func runSeatbeltConfined(*containConfine, []string) int { return containUnsupportedStatus }
