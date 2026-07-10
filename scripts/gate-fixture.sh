#!/usr/bin/env bash
# gate-fixture.sh <fixture> — objective gate for `bashy supervise`.
# Exit 0 iff the single bash-5.3 fixture PASSes; non-zero otherwise.
#
# Safe by construction:
#   * ONE fixture (never test-bash-parallel — that OOM-crashed the box).
#   * backgrounded + reaped: `make test-bash` hangs on exit after printing
#     Results (stray per-fixture timers hold the pipe), so we poll for the
#     verdict line and kill make rather than wait for it.
#   * refuses coproc/jobs/trap: they infinite-loop and orphan runaway
#     `trap9.sub` processes that escape the timeout — verify those on the
#     dedicated remote host, not here.
set -u
fix="${1:?usage: gate-fixture.sh <fixture>}"
case " coproc jobs trap " in
  *" $fix "*)
    echo "gate: '$fix' hangs headless and orphans runaways — verify it on the remote host, not locally" >&2
    exit 2 ;;
esac
cd "$(dirname "$0")/.." || exit 2

log=$(mktemp)
trap 'rm -f "$log"' EXIT
make test-bash TESTS="$fix" >"$log" 2>&1 &
mp=$!
for _ in $(seq 1 150); do
  grep -qE "^  (PASS|FAIL|TIME|SKIP)  $fix\$" "$log" && break
  kill -0 "$mp" 2>/dev/null || break
  sleep 1
done
kill "$mp" 2>/dev/null; wait "$mp" 2>/dev/null

if grep -qE "^  PASS  $fix\$" "$log"; then
  echo "gate: $fix PASS"
  exit 0
fi
echo "gate: $fix NOT passing —"
grep -E "^  (FAIL|TIME|SKIP)  $fix\$|^Results:" "$log" | sed 's/^/  /'
exit 1
