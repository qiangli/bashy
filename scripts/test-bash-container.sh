#!/usr/bin/env bash
# Run the complete GNU Bash 5.3 fixture gate from a self-contained OCI image.
# No checkout or fixture mount is visible to the runtime container.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO"

ARCH="${BASH53_ARCH:-$(go env GOARCH)}"
IMAGE="${BASH53_IMAGE:-localhost/bash53-conformance-hermetic:latest}"
OCI="${BASH53_OCI:-bashy podman}"

# On macOS, use Bashy's private Podman Machine API socket directly. This keeps
# the hermetic lane independent of unrelated or malformed entries in the
# operator's ~/.ssh/known_hosts while retaining the private-machine boundary.
if [ -z "${CONTAINER_HOST:-}" ] && [ "$(go env GOOS)" = darwin ]; then
  machine_socket="$({ $OCI machine inspect bashy 2>/dev/null | sed -n 's/^[[:space:]]*"Path": "\([^"]*api\.sock\)".*/\1/p' | head -1; } || true)"
  if [ -n "$machine_socket" ] && [ -S "$machine_socket" ]; then
    export CONTAINER_HOST="unix://$machine_socket"
  fi
fi

case "$ARCH" in
  amd64|arm64) ;;
  *)
    echo "test-bash-container: unsupported Linux container architecture: $ARCH" >&2
    exit 2
    ;;
esac

echo ">> building self-contained Bash 5.3 conformance image $IMAGE (linux/$ARCH)" >&2
$OCI info >/dev/null
OCI="$OCI" SUITE=bash53 ARCH="$ARCH" IMAGE="$IMAGE" SELFCHECK=inventory \
  scripts/build-conformance-image.sh

echo ">> running all 86 fixtures in the hermetic image" >&2
# --tty supplies a controlling terminal for read/test/vredir. A fresh tmpfs
# prevents fixed upstream names such as /tmp/bash from colliding with host or
# prior-run state. The baked fixture tree and binaries remain immutable.
$OCI run --rm --platform "linux/$ARCH" \
  --user 1000:1000 \
  --tty \
  --network none \
  --read-only \
  --tmpfs /tmp:rw,exec,mode=1777 \
  --tmpfs /var/tmp:rw,exec,mode=1777 \
  -e BASH53_ARCH="$ARCH" \
  -e BASH53_RUNNER="hermetic-$ARCH" \
  -e BASH53_TIMEOUT="${BASH53_TIMEOUT:-60s}" \
  -e BASH53_JOBS_TIMEOUT="${BASH53_JOBS_TIMEOUT:-120s}" \
  -e BASH53_MEM_KB="${BASH53_MEM_KB:-4194304}" \
  "$IMAGE" \
  sigdfl "./bin/bash53suite-linux-$ARCH" \
    -tests-dir /bash53/tests \
    -bash "./bin/bash-linux-$ARCH/bash" \
    -tests "${TESTS:-}"
