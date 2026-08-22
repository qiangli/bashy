#!/bin/sh
set -eu

repo=$(CDPATH= cd -P "$(dirname "$0")/../.." && pwd)
parent=$(dirname "$repo")
image=${BASHY_OCI_IMAGE:-localhost/bashy-base:ubuntu24.04}
action=${1:-all}

fail() { echo "build-smoke: $*" >&2; exit 1; }

if [ -n "${BASHY_OCI_PLATFORM:-}" ]; then
  platform=$BASHY_OCI_PLATFORM
else
  case "$(uname -m)" in
    x86_64|amd64) native_arch=amd64 ;;
    arm64|aarch64) native_arch=arm64 ;;
    *) fail "unsupported native architecture: $(uname -m); set BASHY_OCI_PLATFORM" ;;
  esac
  platform=linux/$native_arch
fi

case "$action" in
  build|smoke|all) ;;
  *) fail "usage: $0 [build|smoke|all]" ;;
esac

if [ -n "${BASHY_OCI:-}" ]; then
  oci=$BASHY_OCI
elif command -v podman >/dev/null 2>&1; then
  oci=podman
elif command -v docker >/dev/null 2>&1; then
  oci=docker
else
  fail "need podman or docker (or set BASHY_OCI to its executable path)"
fi

stage_tree() {
  src=$1
  dest=$2
  [ -d "$src" ] || fail "required sibling checkout is missing: $src"
  git -C "$src" rev-parse --is-inside-work-tree >/dev/null 2>&1 ||
    fail "required sibling is not a git checkout: $src"
  mkdir -p "$dest"
  # Only the four open-source build inputs enter the OCI context. In particular,
  # no umbrella sibling outside this explicit list—or ignored personal/cache
  # content inside one of them—can be sent to the daemon. Include untracked,
  # non-ignored files so a worktree can test this recipe before committing it.
  (cd "$src" && git ls-files --cached --others --exclude-standard -z |
    tar --null -T - -cf -) | (cd "$dest" && tar -xf -)
}

build_image() {
  context=$(mktemp -d "${TMPDIR:-/tmp}/bashy-oci.XXXXXX")
  trap 'rm -rf "$context"' EXIT HUP INT TERM
  stage_tree "$repo" "$context/bashy"
  stage_tree "$parent/coreutils" "$context/coreutils"
  stage_tree "$parent/sh" "$context/sh"
  stage_tree "$parent/readline" "$context/readline"

  "$oci" build \
    --platform "$platform" \
    --file "$context/bashy/tools/bashy-oci/Containerfile" \
    --tag "$image" \
    --build-arg "VERSION=${VERSION:-dev}" \
    --build-arg "BUILD_ID=${BUILD_ID:-oci}" \
    "$context"
  rm -rf "$context"
  trap - EXIT HUP INT TERM
}

run_isolated() {
  "$oci" run --rm --pull=never --network=none --read-only \
    --cap-drop=ALL --security-opt=no-new-privileges \
    --tmpfs /tmp:rw,nosuid,nodev,size=32m,mode=1777 \
    --tmpfs /work:rw,nosuid,nodev,size=32m,mode=0755 \
    "$@"
}

smoke_image() {
  run_isolated "$image" --version >/dev/null
  run_isolated "$image" -c 'printf "%s\n" bashy-oci-smoke' |
    grep -qx 'bashy-oci-smoke' || fail "Bashy command smoke failed"
  run_isolated --entrypoint /bin/sh "$image" -c '
    test -x /bashy
    test -x /bashy.real
    test ! /bin/sh -ef /bashy
    test "$(readlink /opt/bashy/bin/sh)" = /bashy
    test "$OTEL_TRACES_EXPORTER" = none
    /opt/bashy/bin/sh -c '\''
      test "$0" = /opt/bashy/bin/sh
      test "${POSIXLY_CORRECT+x}" = x
      printf "%s\n" posix-alias-ok
    '\''
  ' | grep -qx 'posix-alias-ok' || fail "launcher pair or sh compatibility smoke failed"
  echo "build-smoke: PASS ($image, $platform)"
}

[ "$action" = smoke ] || build_image
[ "$action" = build ] || smoke_image
