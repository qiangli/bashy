#!/bin/sh
# Run the sealed offline self-hosting fixed-point release gate:
#   Stage 0 (previous bashy -> candidate_0)
#   Stage 1 (candidate_0    -> candidate_1)
#   Stage 2 (candidate_1    -> candidate_2)
# Verifies image.lock pins first and emits the bashy-selfhost-fixedpoint-v1 JSON
# report either inside the locked build image (--container, BASHY_BUILD_OFFLINE=1)
# or natively on target-OS runners (--native: Linux, macOS, Windows).
set -eu

here=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -P "$here/../.." && pwd)
lock=${BASHY_BUILD_LOCK:-$here/image.lock}

fail() { echo "bashy-build fixedpoint-offline: $*" >&2; exit 1; }
get_lock() { sed -n "s/^$1=//p" "$lock" | head -n 1; }

mode=${BASHY_FIXEDPOINT_MODE:-native}
source_dir=$repo_root
pkg=./cmd/bashy
prev_bashy=${BASHY_PREV_EXE:-}
artifact_root=${BASHY_ARTIFACTS_DIR:-}
loom_dir=${BASHY_LOOM_DIR:-}
version=
build_id=
output_binary=

while [ $# -gt 0 ]; do
  case "$1" in
    --container) mode=container; shift ;;
    --native) mode=native; shift ;;
    --mode) [ $# -ge 2 ] || fail "missing --mode value"; mode=$2; shift 2 ;;
    --lock) [ $# -ge 2 ] || fail "missing --lock value"; lock=$2; shift 2 ;;
    --source) [ $# -ge 2 ] || fail "missing --source value"; source_dir=$2; shift 2 ;;
    --pkg) [ $# -ge 2 ] || fail "missing --pkg value"; pkg=$2; shift 2 ;;
    --prev-bashy) [ $# -ge 2 ] || fail "missing --prev-bashy value"; prev_bashy=$2; shift 2 ;;
    --artifact-root) [ $# -ge 2 ] || fail "missing --artifact-root value"; artifact_root=$2; shift 2 ;;
    --loom-dir) [ $# -ge 2 ] || fail "missing --loom-dir value"; loom_dir=$2; shift 2 ;;
    --version) [ $# -ge 2 ] || fail "missing --version value"; version=$2; shift 2 ;;
    --build-id) [ $# -ge 2 ] || fail "missing --build-id value"; build_id=$2; shift 2 ;;
    --output-binary) [ $# -ge 2 ] || fail "missing --output-binary value"; output_binary=$2; shift 2 ;;
    --help|-h)
      cat <<EOF
usage: fixedpoint-offline.sh [--native | --container] [--lock FILE] [--source DIR]
                             [--prev-bashy BIN] [--artifact-root DIR] [--loom-dir DIR]
                             [--pkg PKG] [--version VER] [--build-id ID] [--output-binary PATH]
EOF
      exit 0
      ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[ -f "$lock" ] || fail "missing image.lock: $lock"
for key in bashy_version go_version go_sha256_amd64 go_sha256_arm64 base_image base_digest image image_digest; do
  val=$(get_lock "$key")
  [ -n "$val" ] || fail "image.lock missing required key: $key"
done

case "$(get_lock base_digest)" in sha256:*) ;; *) fail "invalid base_digest in $lock" ;; esac
case "$(get_lock image_digest)" in sha256:*) ;; *) fail "invalid image_digest in $lock" ;; esac

source_abs=$(CDPATH= cd -P "$source_dir" && pwd) || fail "no source directory: $source_dir"
[ -f "$source_abs/go.mod" ] || fail "$source_abs has no go.mod"

lock_dir=$(CDPATH= cd -P "$(dirname "$lock")" && pwd) || fail "no lock directory: $lock"
lock_abs="$lock_dir/$(basename "$lock")"

[ -n "$version" ] || version=$(get_lock bashy_version)
if [ -z "$build_id" ]; then
  if [ -f "$source_abs/commits.txt" ]; then
    build_id=$(sed -n 's/^bashy_commit=//p' "$source_abs/commits.txt" | cut -c1-7)
  fi
  if [ -z "$build_id" ]; then
    src_commit=$(get_lock source_bashy_commit)
    [ -n "$src_commit" ] && build_id=$(printf '%s' "$src_commit" | cut -c1-7)
  fi
fi

case "$mode" in
  container)
    if [ -n "$artifact_root" ]; then
      export BASHY_ARTIFACTS_MOUNT="$artifact_root"
    fi
    export BASHY_BUILD_OFFLINE=1
    export BASHY_BUILD_LOCK="$lock_abs"
    in_prev=${prev_bashy:-/bashy}
    in_art=/var/lib/bashy/artifacts
    cmd_str="bashy self fixed-point --json --prev-bashy '$in_prev' --source /work --pkg '$pkg' --version '$version'"
    [ -n "$build_id" ] && cmd_str="$cmd_str --build-id '$build_id'"
    [ -n "$artifact_root" ] && cmd_str="$cmd_str --artifact-root '$in_art'"
    if [ -f "$source_abs/tools/bashy-build/image.lock" ]; then
      cmd_str="$cmd_str --lock /work/tools/bashy-build/image.lock"
    fi
    exec "$here/run.sh" "$source_abs" -- "$cmd_str"
    ;;
  native)
    runner_bin=${BASHY_EXE:-${prev_bashy:-bashy}}
    set -- self fixed-point --json \
      --source "$source_abs" \
      --pkg "$pkg" \
      --lock "$lock_abs" \
      --version "$version"
    [ -n "$prev_bashy" ] && set -- "$@" --prev-bashy "$prev_bashy"
    [ -n "$build_id" ] && set -- "$@" --build-id "$build_id"
    [ -n "$artifact_root" ] && set -- "$@" --artifact-root "$artifact_root"
    [ -n "$loom_dir" ] && set -- "$@" --loom-dir "$loom_dir"
    [ -n "$output_binary" ] && set -- "$@" --output-binary "$output_binary"
    export BASHY_OFFLINE=1
    exec "$runner_bin" "$@"
    ;;
  *)
    fail "unsupported --mode $mode (want native or container)"
    ;;
esac
