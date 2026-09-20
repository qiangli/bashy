#!/bin/sh
# stage-closure.sh ROOTFS FILE...
#
# Copy every shared library — and the ELF loader — that each FILE needs, as
# reported by ldd, into ROOTFS at its original absolute path. Used by
# examples/quickstart/Containerfile to make a `FROM scratch` image runnable:
#
#   - bashy's Linux build is NOT fully static even with CGO_ENABLED=0:
#     coreutils' locale gate (pkg/ctype, pkg/collate) dlopens glibc through
#     ebitengine/purego, whose fakecgo emits cgo_import_dynamic directives,
#     so the binary requests /lib/ld-linux-*.so and libc/libdl/libpthread.
#   - the python-build-standalone interpreter in mode-c needs its own closure.
#
# Symlinks are dereferenced on copy (cp -L), so the loader path the binary
# requests exists as a real file. Files already under ROOTFS (libpython via
# rpath) are skipped. Non-ELF inputs are ignored (ldd's error is discarded).
set -eu
rootfs=$1; shift
for f; do ldd "$f" 2>/dev/null || true; done \
  | awk '/=>/ {print $3} /ld-linux|ld64/ {print $1}' \
  | sort -u \
  | while read -r lib; do
      case "$lib" in ""|"$rootfs"/*) continue;; esac
      [ -f "$lib" ] || continue
      mkdir -p "$rootfs$(dirname "$lib")"
      cp -aL "$lib" "$rootfs$lib"
    done
