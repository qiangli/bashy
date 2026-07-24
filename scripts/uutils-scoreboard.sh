#!/usr/bin/env bash
# Run the foreign uutils suite only inside a disposable, resource-capped OCI
# container. The host prepares an immutable source archive and a read-only SUT;
# only the requested result directory is writable from the container.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT=$PWD
# shellcheck source=scripts/uutils-oci-lib.sh
. "$ROOT/scripts/uutils-oci-lib.sh"

LIST=0
if [ "${1:-}" = --list ]; then
  LIST=1
  shift
fi
OUT=${1:-${UUTILS_OUT:-/tmp/uutils-scoreboard}}
UU=${UUTILS:-$ROOT/../coreutils/reference/uutils-coreutils}
THREADS=${THREADS:-2}
IMAGE=${UUTILS_OCI_IMAGE:-localhost/bashy-uutils-cert:local}
MEMORY=${UUTILS_MEMORY:-3g}
PIDS=${UUTILS_PIDS:-512}
TIMEOUT=${UUTILS_TIMEOUT:-3600}

case "$THREADS:$PIDS:$TIMEOUT" in
  *[!0-9:]*|:*|*::*|*:) echo "THREADS, UUTILS_PIDS, and UUTILS_TIMEOUT must be positive integers" >&2; exit 2 ;;
esac
[ "$THREADS" -gt 0 ] && [ "$PIDS" -gt 0 ] && [ "$TIMEOUT" -gt 0 ] || {
  echo "THREADS, UUTILS_PIDS, and UUTILS_TIMEOUT must be positive integers" >&2
  exit 2
}

[ -d "$UU/tests/by-util" ] || {
  echo "uutils clone not found at $UU" >&2
  exit 2
}
git -C "$UU" rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
  echo "uutils input must be a git checkout so an immutable tracked-file archive can be made" >&2
  exit 2
}

if [ -n "${SUT:-}" ]; then
  [ -x "$SUT" ] || { echo "SUT not executable: $SUT" >&2; exit 2; }
else
  echo "building ../coreutils multicall on the host (the foreign suite remains contained)---" >&2
  ( cd ../coreutils && go build -trimpath -o bin/coreutils ./cmd/coreutils )
  SUT=$(cd ../coreutils && pwd)/bin/coreutils
fi
SUT=$(cd "$(dirname "$SUT")" && pwd)/$(basename "$SUT")

resolve_uutils_oci
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
# Never let a failed new attempt masquerade behind a previous complete result.
rm -f "$OUT/run.txt" "$OUT/failures.txt"
uid=$(id -u)
gid=$(id -g)
[ "$uid" -ne 0 ] || uid=65534
[ "$gid" -ne 0 ] || gid=65534
WORK=$(mktemp -d "${TMPDIR:-/tmp}/bashy-uutils.XXXXXX")
ARCHIVE=$WORK/uutils.tar
CIDFILE=$WORK/cid
TIMED_OUT=$WORK/timed-out
CLIENT_LOG=$OUT/container-client.txt
SCORE_TMP=$WORK/scoreboard
FAIL_TMP=$WORK/failures
CONTAINER_OUT=$WORK/out
CONTAINER_NAME="bashy-uutils-${uid}-$$"

cleanup() {
  if [ -s "$CIDFILE" ]; then
    "${UUTILS_OCI_CMD[@]}" rm -f "$(cat "$CIDFILE")" >/dev/null 2>&1 || true
  fi
  "${UUTILS_OCI_CMD[@]}" rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT INT TERM HUP

# git archive excludes target/, .git/, credentials, and untracked host files.
git -C "$UU" archive --format=tar -o "$ARCHIVE" HEAD

mkdir "$CONTAINER_OUT"
# Root-run stewards are deliberately mapped to nobody in the container.
chmod 0777 "$CONTAINER_OUT"
build_uutils_oci_args \
  "$IMAGE" "$uid:$gid" "$MEMORY" "$PIDS" "$CIDFILE" "$CONTAINER_NAME" \
  "$ARCHIVE" "$SUT" "$ROOT/scripts/uutils-scoreboard-inner.sh" "$CONTAINER_OUT" "$THREADS"

echo "running uutils suite in disposable OCI container via ${UUTILS_OCI_CMD[*]}---" >&2
set +e
"${UUTILS_OCI_CMD[@]}" "${UUTILS_OCI_ARGS[@]}" >"$CLIENT_LOG" 2>&1 &
client_pid=$!
(
  sleep "$TIMEOUT"
  if kill -0 "$client_pid" 2>/dev/null; then
    : >"$TIMED_OUT"
    kill -TERM "$client_pid" 2>/dev/null || true
    sleep 5
    kill -KILL "$client_pid" 2>/dev/null || true
  fi
) &
watchdog_pid=$!
wait "$client_pid"
container_rc=$?
kill "$watchdog_pid" 2>/dev/null || true
wait "$watchdog_pid" 2>/dev/null
set -e

if [ -e "$TIMED_OUT" ]; then
  echo "uutils-scoreboard: container exceeded ${TIMEOUT}s; NO scoreboard emitted" >&2
  exit 2
fi

RAW=$CONTAINER_OUT/run.txt
if ! "$ROOT/scripts/uutils-scoreboard-parse.sh" \
  "$RAW" "$FAIL_TMP" "$container_rc" >"$SCORE_TMP"; then
  echo "uutils-scoreboard: incomplete/aborted run (container exit $container_rc); NO scoreboard emitted" >&2
  exit 2
fi

mv "$FAIL_TMP" "$OUT/failures.txt"
cp "$RAW" "$OUT/run.txt.tmp.$$"
mv "$OUT/run.txt.tmp.$$" "$OUT/run.txt"
cat "$SCORE_TMP"
echo "full run: $OUT/run.txt ; failing cases: $OUT/failures.txt" >&2
[ "$LIST" -eq 0 ] || cat "$OUT/failures.txt"
