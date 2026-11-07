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
cache_volume=bashy-self-cache-$lane
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
"${oci[@]}" volume create "$cache_volume" >/dev/null
trap '"${oci[@]}" volume rm "$cache_volume" >/dev/null 2>&1 || true' EXIT
# Resolve the pinned Go module and provision Node/corepack/pnpm while the
# container has network access. The same cache is mounted into the offline run.
"${oci[@]}" run --rm --name "$container-warm" \
  -e GOWORK=off -e GOMODCACHE=/cache/gomod -e XDG_CACHE_HOME=/cache -e COREPACK_HOME=/cache/corepack \
  -e BASHY_BIN_CACHE=/cache/bashy \
  -e npm_config_store_dir=/cache/pnpm-store -e pnpm_config_store_dir=/cache/pnpm-store \
  -e npm_config_offline=false -e pnpm_config_offline=false \
  -v "$cache_volume:/cache" -v "$dhnt/bashy:/source/bashy:ro" \
  "$image" bash -lc '
    set -euo pipefail
    cp -a /source/bashy/. /work/bashy/
    cd /work/bashy
    go mod download all
    # Nested example modules (desk kit, brainstorm server, …) are built by
    # tests that later run offline; resolve their dependencies now.
    # Nested modules say "go 1.27" with no toolchain line; use the root
    # module'"'"'s toolchain so the image go does not try to fetch it. Any failure
    # fails the warm-up.
    root_tc=$(awk '"'"'$1=="toolchain"{print $2}'"'"' go.mod)
    [ -n "$root_tc" ] || root_tc=$(go env GOVERSION)
    export GOTOOLCHAIN=$root_tc
    find . -name go.mod -not -path ./go.mod -not -path "./builds/*" -not -path "*/node_modules/*" |
      while read -r mod; do
        (cd "$(dirname "$mod")" && go mod download all) || { echo "warm-up: go mod download failed in $mod" >&2; exit 1; }
      done
    unset GOTOOLCHAIN
    mkdir -p bin
    go build -o bin/bashy-spa-check ./cmd/bashy
    # Provision bashy'"'"'s own Go SDK into the mounted cache so the offline
    # transpile/go-source tests find it without fetching the release index.
    # Invoke the front door directly: PATH manipulation does not prevent Go
    # GOTOOLCHAIN switching from downloading into the module cache.
    GOTOOLCHAIN=local /work/bashy/bin/bashy-spa-check go version
    test -x /cache/bashy/go/1.27.2/go/bin/go
    test -f /cache/bashy/go/1.27.2/.binmgr-tree-complete
    yoke_dir=$(go list -m -f "{{.Dir}}" github.com/qiangli/yoke)
    cp -a "$yoke_dir/pkg/meet/web" /work/web
    chmod -R u+w /work/web
    cd /work/web
    /work/bashy/bin/bashy-spa-check pnpm install --frozen-lockfile
  '
# Tests run WITH network, as in CI: shaped-app checks declare
# @effects(...,net) and the shape verifier gives each check a fresh, isolated
# toolchain and module cache by design. The warm caches above only speed the
# run up; pnpm still installs offline from the warmed store.
"${oci[@]}" run --rm --name "$container" --label "io.dhnt.test.lane=$lane" \
  --cpus "${BASHY_TEST_CPUS:-2}" --memory "${BASHY_TEST_MEMORY:-4g}" --pids-limit 4096 \
  -e GOTOOLCHAIN=auto -e GOWORK=off -e GOMODCACHE=/cache/gomod -e XDG_CACHE_HOME=/cache \
  -e COREPACK_HOME=/cache/corepack -e BASHY_BIN_CACHE=/cache/bashy \
  -e npm_config_store_dir=/cache/pnpm-store -e pnpm_config_store_dir=/cache/pnpm-store \
  -e npm_config_offline=true -e pnpm_config_offline=true \
  -e "BASHY_TEST_TARGET=$target" \
  -v "$cache_volume:/cache" \
  -v "$dhnt/bashy:/source/bashy:ro" -v "$dhnt/sh:/source/sh:ro" \
  -v "$dhnt/coreutils:/source/coreutils:ro" -v "$dhnt/readline:/source/readline:ro" \
  -v "$dhnt/filebrowser:/source/filebrowser:ro" \
  "$image" bash -lc '
    set -euo pipefail
    cp -a /source/bashy/. /work/bashy/
    mkdir -p /work/sh /work/coreutils /work/readline /work/filebrowser
    cp -a /source/sh/. /work/sh/
    cp -a /source/coreutils/. /work/coreutils/
    cp -a /source/readline/. /work/readline/
    cp -a /source/filebrowser/. /work/filebrowser/
    cd /work/bashy
    mkdir -p bin
    go build -o bin/bashy ./cmd/bashy
    make "$BASHY_TEST_TARGET"
  '
