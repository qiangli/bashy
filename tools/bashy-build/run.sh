#!/bin/sh
# The one entry point for builds and tests: run a command in the official
# build image named by tools/bashy-build/image.lock, through bashy oci.
#
#   tools/bashy-build/run.sh SRC_DIR -- COMMAND...
#
# SRC_DIR is mounted at /work (chowned to the image's unprivileged user).
# Go module and build caches live in a named volume on this runner and are
# reused by every run, so nothing is downloaded twice.
set -eu
here=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
lock=${BASHY_BUILD_LOCK:-$here/image.lock}
fail() { echo "bashy-build run: $*" >&2; exit 1; }
get() { sed -n "s/^$1=//p" "$lock" | head -n 1; }

[ $# -ge 3 ] && [ "$2" = "--" ] || fail "usage: run.sh SRC_DIR -- COMMAND..."
src=$(CDPATH= cd -P "$1" && pwd) || fail "no source directory: $1"
shift 2

image=$(get image)
digest=$(get image_digest)
[ -n "$image" ] || fail "image.lock names no image"
ref=$image
[ -n "$digest" ] && ref=$image@$digest

oci=${BASHY_OCI:-bashy oci}
volume=${BASHY_BUILD_CACHE_VOLUME:-bashy-build-cache}
$oci volume exists "$volume" 2>/dev/null || $oci volume create "$volume" >/dev/null

exec $oci run --rm \
  -v "$volume:/var/cache/bashy-build:U" \
  -v "$src:/work:U" \
  -w /work \
  -e BASHY_BUILD_RUN=1 \
  "$ref" -c "$*"
