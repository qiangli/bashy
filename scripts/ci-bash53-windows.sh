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
# Runs under Git for Windows' bash on a GitHub windows runner (`shell: bash`),
# which is also where the fixtures' userland comes from: the pure drop-in
# carries no cat/sed/awk/diff, so BASH53_TOOLS_PATH names Git's usr\bin unless
# the caller already chose one. BASH53_TIMEOUT bounds a hung fixture (default
# 60s here; a fixture that waits on a tty costs one timeout, not the budget).
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
echo "bash53-windows: building the pure drop-in and the fixture runner"
CGO_ENABLED=0 go build -o bin/bash.exe ./cmd/bash || exit 2
go build -o bin/bash53suite.exe ./tools/bash53suite || exit 2
version=$(./bin/bash.exe --version | head -1)
echo "bash53-windows: testee: $version"

echo "bash53-windows: ensuring the pinned bash-5.3 fixture corpus"
tree=$(go run ./tools/bash53fixtures -root .) || exit 2
[ -d "$tree/tests" ] || { echo "bash53-windows: no tests/ under $tree" >&2; exit 2; }

if [ -z "${BASH53_TOOLS_PATH:-}" ]; then
  # Git for Windows' MSYS userland, in the OS spelling the harness hands to
  # the fixture PATH (cygpath is Git's own; fall back to the default install).
  BASH53_TOOLS_PATH=$(cygpath -w /usr/bin 2>/dev/null || printf '%s' 'C:\Program Files\Git\usr\bin')
fi
export BASH53_TOOLS_PATH
echo "bash53-windows: fixture userland: $BASH53_TOOLS_PATH"
tools_unix=$(cygpath -u "$BASH53_TOOLS_PATH" 2>/dev/null || printf '%s' "$BASH53_TOOLS_PATH")
for t in cat sed awk diff; do
  [ -x "$tools_unix/$t.exe" ] || [ -x "$tools_unix/$t" ] \
    || echo "bash53-windows: note: $t not found under BASH53_TOOLS_PATH; fixtures calling it will fail" >&2
done
for c in cc gcc clang; do
  command -v "$c" >/dev/null 2>&1 && { echo "bash53-windows: helper compiler: $(command -v "$c")"; break; }
done

export BASH53_TIMEOUT="${BASH53_TIMEOUT:-60s}"
export BASH53_JOBS_TIMEOUT="${BASH53_JOBS_TIMEOUT:-120s}"
log=bash53-windows.log
listed=$(./bin/bash53suite.exe -tests-dir "$tree/tests" -list | grep -c .)
echo "bash53-windows: $listed fixtures in the corpus; running (per-fixture timeout $BASH53_TIMEOUT)"
./bin/bash53suite.exe -tests-dir "$tree/tests" -bash bin/bash.exe ${TESTS:+-tests "$TESTS"} 2>&1 | tee "$log"
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

printf '{"os":"windows","listed":%d,"runnable":%d,"passed":%d,"failed":%d,"timed_out":%d,"skipped":%d,"testee":"%s","tools_path":"%s","per_fixture_timeout":"%s"}\n' \
  "$listed" "$runnable" "$passed" "$failed" "$timed" "$skipped" "$version" "${BASH53_TOOLS_PATH//\\/\\\\}" "$BASH53_TIMEOUT" \
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
  echo "Testee: the pure \`bin/bash.exe\` drop-in; fixture userland: \`$BASH53_TOOLS_PATH\`; per-fixture timeout $BASH53_TIMEOUT."
  echo
  echo "Failing / timed-out fixtures:"
  echo
  echo '```'
  grep -E '^  (FAIL|TIME)  ' "$log" | awk '{print $2}' | sort | tr '\n' ' '; echo
  echo '```'
} | tee bash53-windows-summary.md >> "${GITHUB_STEP_SUMMARY:-/dev/null}"
echo "bash53-windows: $passed/$runnable passed ($failed failed, $timed timed out, $skipped skipped of $listed listed; harness exit $rc — a non-zero exit only says some fixture failed, which is the measurement)"
exit 0
