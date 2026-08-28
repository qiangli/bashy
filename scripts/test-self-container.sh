#!/usr/bin/env bash
# Run Bashy's self build/unit suite in an agent-owned disposable OCI container.
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
dhnt=$(cd "$repo/.." && pwd)
# shellcheck source=scripts/test-lane-id.sh
. "$repo/scripts/test-lane-id.sh"
lane=$(bashy_test_lane "$repo") || { echo 'test-self-container: unsafe BASHY_TEST_LANE' >&2; exit 2; }
oci_text=${BASHY_TEST_OCI:-${BASHY:-bashy} podman}
read -r -a oci <<<"$oci_text"
base=${BASHY_TEST_BASE_IMAGE:-docker.io/library/ubuntu@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517}
containerfile=${BASHY_TEST_CONTAINERFILE:-$repo/tools/dev-test-container/Containerfile}
image=${BASHY_TEST_IMAGE:-localhost/bashy-self-test:$lane}
container=bashy-self-$lane
target=${BASHY_TEST_TARGET:-test}
case "$target" in ''|*[!A-Za-z0-9_.-]*) echo "test-self-container: unsafe BASHY_TEST_TARGET: $target" >&2; exit 2 ;; esac

# Bashy's private Podman machine must not depend on unrelated entries in the
# operator's SSH known_hosts file. Use its API socket directly on macOS.
if [ -z "${CONTAINER_HOST:-}" ] && [ "$(uname -s)" = Darwin ]; then
  machine_socket="$({ "${oci[@]}" machine inspect bashy 2>/dev/null |
    sed -n 's/^[[:space:]]*"Path": "\([^"]*api\.sock\)".*/\1/p' | head -1; } || true)"
  if [ -n "$machine_socket" ] && [ -S "$machine_socket" ]; then
    export CONTAINER_HOST="unix://$machine_socket"
  fi
fi

for path in bashy sh coreutils readline filebrowser; do
  [ -d "$dhnt/$path" ] || { echo "test-self-container: missing sibling $dhnt/$path" >&2; exit 2; }
done
"${oci[@]}" container exists "$container" && {
  echo "test-self-container: lane is already owned by container $container" >&2
  exit 2
}

"${oci[@]}" build --pull=missing --build-arg "BASE_IMAGE=$base" -t "$image" -f "$containerfile" \
  "$repo/tools/dev-test-container"
"${oci[@]}" run --rm --name "$container" --label "io.dhnt.test.lane=$lane" \
  --cpus "${BASHY_TEST_CPUS:-2}" --memory "${BASHY_TEST_MEMORY:-4g}" --pids-limit 4096 \
  -e GOTOOLCHAIN=auto -e "BASHY_TEST_TARGET=$target" \
  -v "$dhnt/bashy:/source/bashy:ro" -v "$dhnt/sh:/source/sh:ro" \
  -v "$dhnt/coreutils:/source/coreutils:ro" -v "$dhnt/readline:/source/readline:ro" \
  -v "$dhnt/filebrowser:/source/filebrowser:ro" \
  "$image" bash -lc '
    cp -a /source/bashy/. /work/bashy/
    mkdir -p /work/sh /work/coreutils /work/readline /work/filebrowser
    cp -a /source/sh/. /work/sh/
    cp -a /source/coreutils/. /work/coreutils/
    cp -a /source/readline/. /work/readline/
    cp -a /source/filebrowser/. /work/filebrowser/
    cd /work/bashy
    make "$BASHY_TEST_TARGET"
  '
