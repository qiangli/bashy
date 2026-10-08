#!/bin/sh
# Product-only, pinned Go runtime overlay. Never edits the installed toolchain.
set -eu
case "${1-}" in
 build) ;;
 test|run) echo "go-product: only product builds are supported" >&2; exit 2 ;;
 *) if [ -n "${BASHY_EXE:-}" ]; then exec "$BASHY_EXE" go "$@"; fi; exec go "$@" ;;
esac
# Windows callers may pass scripts\go-product.sh; use / without external tools.
self=$0
while :; do case $self in *\\*) self=${self%%\\*}/${self#*\\} ;; *) break ;; esac; done
root=$(CDPATH= cd -- "$(dirname -- "$self")/.." && pwd)
# Bootstrap the standard-library-only driver for the host, retaining the target
# coordinates separately. The driver restores them for the actual Go command.
# Capture the target before clearing it; no external env(1) (absent on Windows).
target_os=${GOOS-} target_arch=${GOARCH-} target_cgo=${CGO_ENABLED-}
export GOOS= GOARCH= CGO_ENABLED=0
if [ -n "${BASHY_EXE:-}" ]; then
 exec "$BASHY_EXE" go run "$root/tools/productgo/main.go" "$target_os" "$target_arch" "$target_cgo" "$@"
fi
exec go run "$root/tools/productgo/main.go" "$target_os" "$target_arch" "$target_cgo" "$@"
