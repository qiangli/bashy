#!/usr/bin/env bash
# Bounded synthetic tests. Never invokes cargo, uutils, or a real OCI runtime.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT=$PWD
# shellcheck source=scripts/uutils-oci-lib.sh
. "$ROOT/scripts/uutils-oci-lib.sh"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/uutils-scoreboard-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
has_arg() {
  local want=$1 arg
  for arg in "${UUTILS_OCI_ARGS[@]}"; do [ "$arg" != "$want" ] || return 0; done
  return 1
}
has_pair() {
  local want=$1 value=$2 i
  for ((i=0; i+1<${#UUTILS_OCI_ARGS[@]}; i++)); do
    [ "${UUTILS_OCI_ARGS[i]}" != "$want" ] ||
      [ "${UUTILS_OCI_ARGS[i+1]}" != "$value" ] ||
      return 0
  done
  return 1
}

touch "$TMP/source.tar" "$TMP/sut" "$TMP/inner"
mkdir "$TMP/out"
build_uutils_oci_args test-image 501:20 512m 64 "$TMP/cid" test-uutils \
  "$TMP/source.tar" "$TMP/sut" "$TMP/inner" "$TMP/out" 2
for arg in --network=none --read-only --cap-drop=ALL --security-opt=no-new-privileges \
  --pull=never; do
  has_arg "$arg" || fail "OCI command missing $arg"
done
for pair in \
  "--memory 512m" "--memory-swap 512m" "--pids-limit 64" "--user 501:20" \
  "--tmpfs /work:rw,nosuid,nodev,size=8g,mode=1777" \
  "--tmpfs /tmp:rw,nosuid,nodev,size=256m,mode=1777"; do
  has_pair ${pair%% *} "${pair#* }" || fail "OCI command missing pair: $pair"
done
joined=$(printf '%s\n' "${UUTILS_OCI_ARGS[@]}")
case "$joined" in
  *"$HOME"*) fail "OCI command leaked HOME" ;;
esac
case "$joined" in
  *"src=$ROOT,dst="*) fail "OCI command mounted repository root" ;;
esac
case "$joined" in
  *"UUTILS_UNSAFE_"*) fail "supported OCI command exposed unsafe override" ;;
esac
for resolved in \
  test_cp::test_cp_fifo \
  test_cp::test_dir_perm_race_with_preserve_mode_and_ownership \
  test_cat::test_fifo_symlink \
  test_dd::test_random_73k_test_lazy_fullblock \
  test_dd::test_seek_output_fifo \
  test_dd::test_sync_delayed_reader; do
  if grep -Fq -- "--skip $resolved" "$ROOT/scripts/uutils-scoreboard-inner.sh"; then
    fail "resolved case remains quarantined: $resolved"
  fi
done
mounts=0
rw_mounts=0
for ((i=0; i+1<${#UUTILS_OCI_ARGS[@]}; i++)); do
  [ "${UUTILS_OCI_ARGS[i]}" = --mount ] || continue
  mount=${UUTILS_OCI_ARGS[i+1]}
  mounts=$((mounts+1))
  case "$mount" in
    *,dst=/out) rw_mounts=$((rw_mounts+1)); case "$mount" in *,readonly) fail "output unexpectedly readonly" ;; esac ;;
    *,dst=/input/*,readonly) ;;
    *) fail "input mount is not narrowly scoped readonly: $mount" ;;
  esac
done
[ "$mounts" -eq 4 ] || fail "expected four narrow mounts, got $mounts"
[ "$rw_mounts" -eq 1 ] || fail "expected only /out writable"
grep -q 'cargo fetch --locked' "$ROOT/scripts/uutils.Containerfile" ||
  fail "provisioned image does not fetch locked dependencies"
if grep -q 'cargo test' "$ROOT/scripts/uutils.Containerfile"; then
  fail "image preparation must not build or run tests"
fi
grep -q 'export HOME=/work/home' "$ROOT/scripts/uutils-scoreboard-inner.sh" ||
  fail "container HOME is not tmpfs-backed"
grep -q 'export CARGO_HOME=/work/cargo' "$ROOT/scripts/uutils-scoreboard-inner.sh" ||
  fail "container CARGO_HOME is not tmpfs-backed"

complete=$TMP/complete.log
cat >"$complete" <<'EOF'
running 3 tests
test test_cat::ok_case ... ok
test test_cat::bad_case ... FAILED
test test_cp::skip_case ... ignored

test result: FAILED. 1 passed; 1 failed; 1 ignored; 0 measured; 0 filtered out; finished in 0.01s

BASHY_UUTILS_LISTED=3
BASHY_UUTILS_CARGO_EXIT=101
EOF
"$ROOT/scripts/uutils-scoreboard-parse.sh" "$complete" "$TMP/failures" >"$TMP/score"
grep -q '1 pass / 1 fail / 1 ignored' "$TMP/score" || fail "complete scoreboard totals"
grep -q 'test_cat::bad_case' "$TMP/failures" || fail "failure list"

for kind in truncated mismatch preflight aborted trailing; do
  cp "$complete" "$TMP/$kind.log"
done
sed -i.bak '/^test result:/d' "$TMP/truncated.log"
sed -i.bak 's/1 passed; 1 failed/2 passed; 1 failed/' "$TMP/mismatch.log"
sed -i.bak 's/UUTILS_LISTED=3/UUTILS_LISTED=4/' "$TMP/preflight.log"
sed -i.bak 's/CARGO_EXIT=101/CARGO_EXIT=134/' "$TMP/aborted.log"
printf 'unexpected trailing output\n' >>"$TMP/trailing.log"
for kind in truncated mismatch preflight aborted trailing; do
  if "$ROOT/scripts/uutils-scoreboard-parse.sh" "$TMP/$kind.log" "$TMP/nope" >"$TMP/no-score" 2>/dev/null; then
    fail "$kind transcript accepted"
  fi
  [ ! -s "$TMP/no-score" ] || fail "$kind transcript emitted scoreboard"
done

# Stub-OCI integration: prove the launcher passes containment flags and accepts
# a complete ordinary-failure transcript without invoking any foreign code.
mkdir -p "$TMP/uu/tests/by-util"
git -C "$TMP/uu" init -q
git -C "$TMP/uu" config user.email test@example.invalid
git -C "$TMP/uu" config user.name test
touch "$TMP/uu/tests/by-util/placeholder.rs"
git -C "$TMP/uu" add .
git -C "$TMP/uu" commit -qm init
cp /bin/sh "$TMP/fake-sut"
cat >"$TMP/stub-oci" <<'EOF'
#!/usr/bin/env bash
set -eu
[ "${1:-}" != rm ] || exit 0
printf '%s\n' "$@" >"$STUB_LOG"
out=
cid=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --cidfile) cid=$2; shift 2 ;;
    type=bind,src=*,dst=/out) out=${1#type=bind,src=}; out=${out%,dst=/out}; shift ;;
    *) shift ;;
  esac
done
printf stub >"$cid"
if [ "${STUB_MODE:-complete}" = incomplete ]; then
  printf 'cargo aborted before terminal summary\n' >"$out/run.txt"
  exit 125
fi
cat >"$out/run.txt" <<'LOG'
running 1 test
test test_cat::ordinary_failure ... FAILED
test result: FAILED. 0 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.01s
BASHY_UUTILS_LISTED=1
BASHY_UUTILS_CARGO_EXIT=101
LOG
exit 101
EOF
chmod +x "$TMP/stub-oci" "$TMP/fake-sut"
STUB_LOG=$TMP/stub.log UUTILS_OCI=$TMP/stub-oci UUTILS=$TMP/uu SUT=$TMP/fake-sut \
  UUTILS_OUT=$TMP/integration UUTILS_TIMEOUT=10 \
  "$ROOT/scripts/uutils-scoreboard.sh" >"$TMP/integration-score"
grep -q -- '--network=none' "$TMP/stub.log" || fail "stub OCI missed network isolation"
grep -q '0 pass / 1 fail' "$TMP/integration-score" || fail "stub OCI complete failure result"

printf stale >"$TMP/integration/run.txt"
printf stale >"$TMP/integration/failures.txt"
if STUB_MODE=incomplete STUB_LOG=$TMP/stub-incomplete.log \
  UUTILS_OCI=$TMP/stub-oci UUTILS=$TMP/uu SUT=$TMP/fake-sut \
  UUTILS_OUT=$TMP/integration UUTILS_TIMEOUT=10 \
  "$ROOT/scripts/uutils-scoreboard.sh" >"$TMP/incomplete-score" 2>/dev/null; then
  fail "incomplete stub OCI run accepted"
fi
[ ! -e "$TMP/integration/run.txt" ] || fail "stale run.txt survived failed attempt"
[ ! -e "$TMP/integration/failures.txt" ] || fail "stale failures.txt survived failed attempt"
[ ! -s "$TMP/incomplete-score" ] || fail "incomplete OCI run emitted scoreboard"

if "$ROOT/scripts/uutils-scoreboard-parse.sh" \
  "$complete" "$TMP/nope" 125 >"$TMP/rc-mismatch-score" 2>/dev/null; then
  fail "OCI/cargo exit mismatch accepted"
fi
[ ! -s "$TMP/rc-mismatch-score" ] || fail "exit mismatch emitted scoreboard"

echo "uutils scoreboard safety tests: PASS"
