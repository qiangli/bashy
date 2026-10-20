#!/usr/bin/env bash
# Install the pinned, sha256-verified build-only CUDA SDK. Fails closed: any
# digest mismatch aborts before extraction. Usage: install-cuda-toolkit.sh DEST
set -euo pipefail
dest=${1:?dest}
: "${CUDA_TOOLKIT_BASE:?}" "${CUDA_TOOLKIT_SHA256:?}" "${CUDA_TOOLKIT_VERSION:?}"
mkdir -p "$dest" "$dest/.dl"
for entry in $CUDA_TOOLKIT_SHA256; do
  rel=${entry%%=*}; want=${entry##*=}
  [[ $want =~ ^[0-9a-f]{64}$ ]] || { echo "install-cuda-toolkit: bad pin for $rel" >&2; exit 1; }
  f="$dest/.dl/$(basename "$rel")"
  [[ -f $f ]] || curl -fsSL --retry 3 -o "$f" "$CUDA_TOOLKIT_BASE/$rel"
  got=$(sha256sum "$f" | cut -d' ' -f1)
  [[ $got == "$want" ]] || { echo "install-cuda-toolkit: sha256 mismatch for $rel ($got)" >&2; rm -f "$f"; exit 1; }
  tar -xJf "$f" -C "$dest" --strip-components=1
done
printf '%s\n' "$CUDA_TOOLKIT_VERSION" > "$dest/.bashy-toolkit-version"
printf '%s' "$CUDA_TOOLKIT_SHA256" | sha256sum | cut -d' ' -f1 > "$dest/.bashy-toolkit-sha256"
test -x "$dest/bin/nvcc" || { echo 'install-cuda-toolkit: nvcc missing after install' >&2; exit 1; }
