//go:build bashy_cert && bashy_cert_base && !osusergo

package main

// Static glibc can load an NSS module with incompatible process state during
// os/user lookups. The certified one-file build must use Go's account parser;
// fail the build if its osusergo tag is omitted.
var _ = BASHY_CERT_REQUIRES_OSUSERGO_FOR_STATIC_NSS_SAFETY
