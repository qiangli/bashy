//go:build !cgo || (!linux && !darwin)

package main

// The portable release build remains pure Go. Native-parent ignored signal
// dispositions still require the existing siglaunch entry on that build.
func preGoIgnoredSignals() string { return "" }
