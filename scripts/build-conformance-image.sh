#!/usr/bin/env bash
# Build the SELF-CONTAINED bash-5.3 conformance image for k8s/DKS (Argo).
# Bakes the Linux testee + harness + fixture tree (with support/) + chunks.json
# into an image that needs no host mounts. See Containerfile.k8s and
# scripts/dag-to-argo.sh (the tier-5 executor).
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"; cd "$REPO"
ARCH="${ARCH:-amd64}"                                  # cluster node arch
IMAGE="${IMAGE:-localhost/bash53-conformance-k8s:latest}"
OCI="${OCI:-$REPO/bin/bashy podman}"
CTX="$(mktemp -d)/ctx"; mkdir -p "$CTX/bin/bash-linux-$ARCH" "$CTX/bash53"
trap 'rm -rf "$(dirname "$CTX")"' EXIT
log(){ printf '>> %s\n' "$*" >&2; }

log "building linux/$ARCH testee + harness…"
GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build -trimpath -o "$CTX/bin/bash-linux-$ARCH/bash" ./cmd/bash
GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build -trimpath -o "$CTX/bin/bash53suite-linux-$ARCH" ./tools/bash53suite

log "staging fixture tree (strip prebuilt helpers; keep support/*.c)…"
FX="$(readlink external/bash-5.3)"
cp -R "$FX/." "$CTX/bash53/"
rm -f "$CTX/bash53/tests/recho" "$CTX/bash53/tests/zecho" "$CTX/bash53/tests/xcase" "$CTX/bash53/tests/printenv"
cp chunks.json "$CTX/chunks.json"
cp tools/bash53-container/Containerfile.k8s "$CTX/Containerfile"
# the CMD reads $BASH53_ARCH; default it to this build's arch
sed -i.bak "s/^ENV BASH53_ARCH=.*/ENV BASH53_ARCH=$ARCH/" "$CTX/Containerfile" && rm -f "$CTX/Containerfile.bak"

log "building image $IMAGE (linux/$ARCH)…"
$OCI build --platform "linux/$ARCH" -t "$IMAGE" -f "$CTX/Containerfile" "$CTX" 2>&1 | grep -iE 'STEP|COMMIT|Successfully|error' | tail -4

log "done. self-check: run chunk 3 inside the image (no mounts)…"
$OCI run --rm --platform "linux/$ARCH" -e CHUNK=3/8 -e "BASH53_ARCH=$ARCH" "$IMAGE" 2>&1 | grep -aiE 'Results:|FAIL|error' | tail -2
echo "IMAGE=$IMAGE  ARCH=$ARCH" >&2
