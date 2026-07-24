#!/usr/bin/env bash
# Provision the dependency-only image used by the network-disabled suite run.
# This fetches Cargo dependencies but never builds or executes upstream tests.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT=$PWD
. "$ROOT/scripts/uutils-oci-lib.sh"

UU=${UUTILS:-$ROOT/../coreutils/reference/uutils-coreutils}
IMAGE=${UUTILS_OCI_IMAGE:-localhost/bashy-uutils-cert:local}
[ -d "$UU/tests/by-util" ] || { echo "uutils clone not found at $UU" >&2; exit 2; }
git -C "$UU" rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
  echo "uutils input must be a git checkout" >&2
  exit 2
}
resolve_uutils_oci

WORK=$(mktemp -d "${TMPDIR:-/tmp}/bashy-uutils-image.XXXXXX")
trap 'rm -rf "$WORK"' EXIT INT TERM HUP
git -C "$UU" archive --format=tar -o "$WORK/uutils.tar" HEAD
cp "$ROOT/scripts/uutils.Containerfile" "$WORK/Containerfile"

echo "preparing $IMAGE (dependency fetch only; no tests execute)---" >&2
"${UUTILS_OCI_CMD[@]}" build --tag "$IMAGE" --file "$WORK/Containerfile" "$WORK"
echo "prepared $IMAGE" >&2
