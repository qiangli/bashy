#!/bin/sh
# Product-only, pinned Go runtime overlay. Never edits the installed toolchain.
set -eu
case "${1-}" in build|test|run) ;; *) exec go "$@" ;; esac
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# Bootstrap the standard-library-only driver for the host, retaining the target
# coordinates separately. The driver restores them for the actual Go command.
exec env GOOS= GOARCH= CGO_ENABLED=0 go run "$root/tools/productgo/main.go" "${GOOS-}" "${GOARCH-}" "${CGO_ENABLED-}" "$@"
