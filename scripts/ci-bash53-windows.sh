#!/usr/bin/env bash
# ci-bash53-windows.sh — MEASURE the GNU Bash 5.3 fixture suite on Windows.
#
# This is a scoreboard, not a gate. The 86/86 claim belongs to the canonical
# hosts (native macOS serial, the Linux container gate); Windows had never been
# measured at all — the harness failed closed there until job-object
# containment landed (tools/bash53suite/proc_windows.go, Sprint 216). This
# script runs the same runner against the same pinned corpus with the pure
# `bin/bash.exe` drop-in as the testee and publishes the exact counts it
# observed — runnable / passed / failed / timed out / skipped — so the number
# quoted anywhere for Windows is one somebody MEASURED. Never quote 86/86 for
# Windows from anything but this script's summary.
#
# What is a failure of THIS script (exit 2) versus a fixture verdict:
#   * the build, the corpus fetch, or the harness's own preflight fails;
#   * the harness does not print its Results line (an unfinished run must
#     never be scored — fixtures that never ran would look like anything);
#   * no fixture produced a verdict at all.
# A low pass count is NOT a failure here. It is the answer.
#
# Runs under Git for Windows' bash on a GitHub windows runner (`shell: bash`).
# The fixtures' userland is bashy's OWN (Sprint 245): the pure drop-in carries
# no cat/sed/awk/diff, so the harness lays out a POSIX-shaped root
# (BASHY_ROOT: /usr/bin = every applet of the pure-Go yoke multicall binary
# built here, /bin/sh = the testee, /etc/passwd) and puts that usr/bin on the
# fixture PATH — never Git's MSYS usr\bin, whose C runtime and mount table
# are not ours to fix. BASH53_TOOLS_PATH still overrides the userland dir
# when a caller wants to measure against something else; the run header
# prints whatever was used. The corpus helpers (recho/zecho/xcase) are the
# harness binary itself on Windows (tools/bash53suite/helpers.go): no C
# compiler is consulted. BASH53_TIMEOUT bounds a hung fixture (default 60s
# here; a fixture that waits on a tty costs one timeout, not the budget).
#
# Outputs (all in the checkout root):
#   bash53-windows.log          the harness transcript
#   bash53-windows-summary.md   the human summary (also the step summary)
#   bash53-windows-counts.json  {runnable,passed,failed,timed_out,skipped,listed}
set -uo pipefail

cd "$(dirname "$0")/.."
case "$(go env GOOS)" in
windows) ;;
*) echo "bash53-windows: this measures Windows; GOOS=$(go env GOOS)" >&2; exit 2 ;;
esac

mkdir -p bin
echo "bash53-windows: building the pure drop-in, the fixture runner and the userland"
CGO_ENABLED=0 go build -o bin/bash.exe ./cmd/bash || exit 2
go build -o bin/bash53suite.exe ./tools/bash53suite || exit 2
# yoke = coreutils (the certified required set) + the yoke applets (hexdump,
# …), one multicall binary, built in its own module (the flat sibling that
# bootstrap-siblings.sh pinned), so the userland measured is that pin.
[ -d ../yoke ] || { echo "bash53-windows: ../yoke sibling missing (run scripts/bootstrap-siblings.sh)" >&2; exit 2; }
(cd ../yoke && CGO_ENABLED=0 go build -o "$OLDPWD/bin/yoke.exe" ./cmd/yoke) || exit 2
version=$(./bin/bash.exe --version | head -1)
echo "bash53-windows: testee: $version"
echo "bash53-windows: userland: yoke $(./bin/yoke.exe --list | wc -l | tr -d ' ') applets (coreutils $(git -C ../coreutils rev-parse --short HEAD 2>/dev/null || echo pinned), yoke $(git -C ../yoke rev-parse --short HEAD 2>/dev/null || echo pinned))"

echo "bash53-windows: ensuring the pinned bash-5.3 fixture corpus"
tree=$(go run ./tools/bash53fixtures -root .) || exit 2
[ -d "$tree/tests" ] || { echo "bash53-windows: no tests/ under $tree" >&2; exit 2; }

if [ -n "${BASH53_TOOLS_PATH:-}" ]; then
  echo "bash53-windows: BASH53_TOOLS_PATH is set; the fixture PATH uses it INSTEAD of the bashy userland root's usr/bin: $BASH53_TOOLS_PATH"
fi
export BASH53_USERLAND=bin/yoke.exe

export BASH53_TIMEOUT="${BASH53_TIMEOUT:-60s}"
export BASH53_JOBS_TIMEOUT="${BASH53_JOBS_TIMEOUT:-120s}"
log=bash53-windows.log
listed=$(./bin/bash53suite.exe -tests-dir "$tree/tests" -list | grep -c .)
echo "bash53-windows: $listed fixtures in the corpus; running (per-fixture timeout $BASH53_TIMEOUT)"
./bin/bash53suite.exe -tests-dir "$tree/tests" -bash bin/bash.exe -userland "$BASH53_USERLAND" ${TESTS:+-tests "$TESTS"} 2>&1 | tee "$log"
rc=${PIPESTATUS[0]}

results=$(grep '^Results:' "$log" || true)
if [ -z "$results" ]; then
  echo "bash53-windows: INCONCLUSIVE — the harness printed no Results line (exit $rc); nothing is scored." >&2
  exit 2
fi
count() { grep -c "^  $1  " "$log" || true; }
passed=$(count PASS); failed=$(count FAIL); timed=$(count TIME); skipped=$(count SKIP)
runnable=$((passed + failed + timed))
if [ "$runnable" -eq 0 ]; then
  echo "bash53-windows: no fixture verdicts at all (exit $rc); the suite did not run." >&2
  exit 2
fi

tools_used=$(grep -m1 '^Fixture PATH:' "$log" | sed 's/^Fixture PATH: //')
root_used=$(grep -m1 '^Fixture root:' "$log" | sed 's/^Fixture root: //')
printf '{"os":"windows","listed":%d,"runnable":%d,"passed":%d,"failed":%d,"timed_out":%d,"skipped":%d,"testee":"%s","userland":"%s","tools_path":"%s","fixture_root":"%s","per_fixture_timeout":"%s"}\n' \
  "$listed" "$runnable" "$passed" "$failed" "$timed" "$skipped" "$version" "$BASH53_USERLAND" "${tools_used//\\/\\\\}" "${root_used//\\/\\\\}" "$BASH53_TIMEOUT" \
  > bash53-windows-counts.json
{
  echo "## GNU Bash 5.3 fixtures on Windows — measured, not a gate"
  echo
  echo "**$passed / $runnable** runnable fixtures pass on this runner (\`$(uname -sr)\`, $version)."
  echo
  echo "| listed | runnable | passed | failed | timed out | skipped |"
  echo "|---:|---:|---:|---:|---:|---:|"
  echo "| $listed | $runnable | $passed | $failed | $timed | $skipped |"
  echo
  echo '```'
  echo "$results"
  echo '```'
  echo
  echo "Testee: the pure \`bin/bash.exe\` drop-in; userland: bashy's own (\`$BASH53_USERLAND\`, root \`$root_used\`); fixture PATH: \`$tools_used\`; per-fixture timeout $BASH53_TIMEOUT."
  echo
  echo "Failing / timed-out fixtures:"
  echo
  echo '```'
  grep -E '^  (FAIL|TIME)  ' "$log" | awk '{print $2}' | sort | tr '\n' ' '; echo
  echo '```'
} | tee bash53-windows-summary.md >> "${GITHUB_STEP_SUMMARY:-/dev/null}"
echo "bash53-windows: $passed/$runnable passed ($failed failed, $timed timed out, $skipped skipped of $listed listed; harness exit $rc — a non-zero exit only says some fixture failed, which is the measurement)"
exit 0
