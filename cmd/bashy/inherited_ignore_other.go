//go:build !bashy_runtime_snapshot || (!linux && !darwin)

package main

// Builds without the product runtime overlay have no pre-handler snapshot;
// Linux route.go still falls back to the ELF runtime.fwdSig reader.
func preGoIgnoredSignals() string { return "" }
