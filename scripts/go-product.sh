#!/bin/sh
# Product-only, pinned Go runtime overlay. Never edits the installed toolchain.
set -eu
case "${1-}" in
 build) ;;
 test|run) echo "go-product: only product builds are supported" >&2; exit 2 ;;
 *) if [ -n "${BASHY_EXE:-}" ]; then exec "$BASHY_EXE" go "$@"; fi; exec go "$@" ;;
esac
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# Bootstrap the standard-library-only driver for the host, retaining the target
# coordinates separately. The driver restores them for the actual Go command.
if [ -n "${BASHY_EXE:-}" ]; then
 exec env GOOS= GOARCH= CGO_ENABLED=0 "$BASHY_EXE" go run "$root/tools/productgo/main.go" "${GOOS-}" "${GOARCH-}" "${CGO_ENABLED-}" "$@"
fi
exec env GOOS= GOARCH= CGO_ENABLED=0 go run "$root/tools/productgo/main.go" "${GOOS-}" "${GOARCH-}" "${CGO_ENABLED-}" "$@"
