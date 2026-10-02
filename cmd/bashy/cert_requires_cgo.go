//go:build bashy_cert && (!cgo || !linux)

package main

// Profile D must use the pre-Go signal snapshot in the one-file Linux
// executable. Keep other Bashy build profiles available without cgo, but
// fail certification builds instead of silently dropping inherited ignores.
var _ = BASHY_CERT_REQUIRES_CGO_ENABLED_1_ON_LINUX
