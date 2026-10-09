#!/usr/bin/env bash
# windows-bash53-gate.sh — the 86-fixture Bash 5.3 release gate for Windows.
#
# Operator decision D11 (2026-10-09, "bashy is all you need"): on the Windows
# test host there is no Git for Windows, MSYS, WSL, make, or C compiler —
# only Go and this checkout. This script therefore runs UNDER bashy itself
# (which supplies bash 5.3 plus the Go coreutils userland in-process) and
# uses exactly these external programs:
#   go .................. host toolchain (Go 1.27): builds everything below
#   bin/bash.exe ........ the pure drop-in under test (./cmd/bash, no coreutils)
#   bin/bash53suite.exe . the ONE fixture runner (tools/bash53suite)
#   bin/yoke.exe ........ fixture userland (coreutils + applets, BASHY_ROOT)
#   bin/bash53locales.exe  provisions the corpus locale store before the run
#                         (tools/bash53locales: pinned glibc localedata,
#                         compiled with localedef into LOCPATH) so the two
#                         locale-sensitive fixtures measure instead of warn;
#                         the harness repeats the same provisioning when no
#                         host locale provider and no LOCPATH are set
#   zig cc .............. provisioned at run time BY the runner itself for the
#                         one fixture that needs a C compiler (glob-bracket);
#                         never a host compiler (tools/bash53suite/cc_provision.go)
# Everything else in here is a bash builtin or a parameter expansion. In
# particular there is no grep/sed/awk/sort/comm/tee/uname: the log is parsed
# with while-read loops and [[ ]] patterns, so the script cannot depend on a
# foreign MSYS runtime.
#
# The Unix gate this mirrors is `make test-bash` (serial, the release gate):
# Makefile targets test-bash -> build-bash + test-bash-fixtures +
# test-bash-helpers -> test-bash-run (bin/bash53suite -bash bin/bash).
# Its Unix-only scaffolding maps as follows:
#   make ................ this script (serial; no fan-out, no second runner)
#   /bin/bash driver .... bashy.exe itself: bin\bashy.exe <this script>
#   script(1) PTY wrapper  not needed: the Go harness contains hung fixtures
#                         with job objects on Windows (proc_windows.go), the
#                         way it uses Setpgid from the parent on unix
#   cc-built recho/      the harness binary IS recho/zecho/xcase on Windows
#   zecho/xcase ........ (tools/bash53suite/helpers.go)
#   stub config.h etc. . generated inside the harness tree the same way
#   git (corpus fetch) . tools/bash53fixtures (verified user cache, no git)
#
# Usage — from the checkout root on the Windows host, in cmd:
#   bin\bashy.exe scripts/windows-bash53-gate.sh
# Environment:
#   BASH53_RELEASE_DIR .. optional published pair directory (bash.exe + bashy.exe);
#                         builds test helpers only, never product executables
#   TESTS ............... space-separated fixture subset (default: all 86)
#   BASH53_TIMEOUT ...... per-fixture timeout, Go duration (default 60s)
#   BASH53_JOBS_TIMEOUT . jobs-fixture timeout, Go duration (default 180s)
# Outputs (checkout root): windows-bash53-gate.log,
#   windows-bash53-gate-counts.json, windows-bash53-gate-summary.md
# Exit status:
#   0  every listed fixture PASSes — the gate is green.
#   1  measured shortfall: some fixture did not PASS. The counts and the
#      non-passing fixture names are the evidence; the script itself worked.
#   2  inconclusive: a build, the corpus fetch, or the harness preflight
#      failed, the harness printed no Results line, or no verdicts exist.
#      Nothing is scored.
set -u

ROOT="${1:-$PWD}"
cd "$ROOT" || { echo "gate: cannot cd to $ROOT" >&2; exit 2; }

command -v go >/dev/null 2>&1 || { echo "gate: go not on PATH" >&2; exit 2; }
if [ "$(go env GOOS)" != "windows" ]; then
  echo "gate: this gate measures Windows; GOOS=$(go env GOOS)" >&2
  exit 2
fi

log=windows-bash53-gate.log
export BASH53_TIMEOUT="${BASH53_TIMEOUT:-60s}"
export BASH53_JOBS_TIMEOUT="${BASH53_JOBS_TIMEOUT:-180s}"
testee_path=bin/bash.exe
export BASH53_USERLAND=bin/yoke.exe
if [ -n "${BASH53_RELEASE_DIR:-}" ]; then
  testee_path="$BASH53_RELEASE_DIR/bash.exe"
  export BASH53_USERLAND=bin/coreutils.exe
  for member in "$testee_path" "$BASH53_RELEASE_DIR/bashy.exe"; do
    [ -f "$member" ] || { echo "gate: missing release member: $member" >&2; exit 2; }
  done
fi

echo "gate: preparing the testee and building the fixture runner"
mkdir -p bin || exit 2
if [ -z "${BASH53_RELEASE_DIR:-}" ]; then
  CGO_ENABLED=0 go build -o bin/bash.exe ./cmd/bash || exit 2
else
  echo "gate: published product pair: $BASH53_RELEASE_DIR (no product build)"
  # bashy's multicall --list interface is selected by the coreutils alias.
  # Copy the published bytes; the helper must never compile a replacement.
  cp "$BASH53_RELEASE_DIR/bashy.exe" "$BASH53_USERLAND" || exit 2
fi
go build -o bin/bash53suite.exe ./tools/bash53suite || exit 2
testee_full=$("$testee_path" --version)
testee="${testee_full%%$'\n'*}"
echo "gate: testee: $testee"

echo "gate: ensuring the pinned bash-5.3 fixture corpus"
tree=$(go run ./tools/bash53fixtures -root .) || exit 2
[ -d "$tree/tests" ] || { echo "gate: no tests/ under $tree" >&2; exit 2; }

if [ -z "${BASH53_RELEASE_DIR:-}" ]; then
  echo "gate: building the yoke userland at the pinned version"
  GOFLAGS=-mod=mod CGO_ENABLED=0 go build -o bin/yoke.exe github.com/qiangli/yoke/cmd/yoke || exit 2
fi

echo "gate: provisioning the corpus locale store (pinned glibc localedata, compiled with localedef)"
go build -o bin/bash53locales.exe ./tools/bash53locales || exit 2
store=$(./bin/bash53locales.exe) || exit 2
export LOCPATH="$store"
echo "gate: corpus locale store: $store"

listed=0
./bin/bash53suite.exe -tests-dir "$tree/tests" -list >"$log.list" || exit 2
while IFS= read -r _listline; do listed=$((listed + 1)); done <"$log.list"
rm -f "$log.list"
echo "gate: $listed fixtures in the corpus; running (per-fixture timeout $BASH53_TIMEOUT, jobs $BASH53_JOBS_TIMEOUT)"

if [ -n "${TESTS:-}" ]; then
  ./bin/bash53suite.exe -tests-dir "$tree/tests" -bash "$testee_path" \
    -userland "$BASH53_USERLAND" -tests "$TESTS" >"$log" 2>&1
else
  ./bin/bash53suite.exe -tests-dir "$tree/tests" -bash "$testee_path" \
    -userland "$BASH53_USERLAND" >"$log" 2>&1
fi
rc=$?

results=""
passed=0; failed=0; timed=0; skipped=0
bad_list=""
while IFS= read -r line; do
  case "$line" in
    '  PASS  '*) passed=$((passed + 1)) ;;
    '  FAIL  '*)
      failed=$((failed + 1))
      name="${line#'  FAIL  '}"
      name="${name%% *}"
      bad_list="${bad_list}FAIL ${name}"$'\n'
      ;;
    '  TIME  '*)
      timed=$((timed + 1))
      name="${line#'  TIME  '}"
      name="${name%% *}"
      bad_list="${bad_list}TIME ${name}"$'\n'
      ;;
    '  SKIP  '*)
      skipped=$((skipped + 1))
      name="${line#'  SKIP  '}"
      name="${name%% *}"
      bad_list="${bad_list}SKIP ${name}"$'\n'
      ;;
    'Results:'*) results="$line" ;;
  esac
done <"$log"
runnable=$((passed + failed + timed))

if [ -z "$results" ]; then
  echo "gate: INCONCLUSIVE — the harness printed no Results line (exit $rc); nothing is scored." >&2
  exit 2
fi
if [ "$runnable" -eq 0 ]; then
  echo "gate: INCONCLUSIVE — no fixture verdicts at all (exit $rc); the suite did not run." >&2
  exit 2
fi
if [ "$passed" -eq 0 ]; then
  echo "gate: INCONCLUSIVE — 0 fixtures passed; the suite did not run (missing fixtures?). Refusing to score." >&2
  exit 2
fi

goversion=$(go version)
esc_testee="${testee//\\/\\\\}"
esc_goversion="${goversion//\\/\\\\}"
esc_tree="${tree//\\/\\\\}"
printf '{"os":"windows","listed":%d,"runnable":%d,"passed":%d,"failed":%d,"timed_out":%d,"skipped":%d,"testee":"%s","userland":"%s","fixture_tree":"%s","go":"%s","per_fixture_timeout":"%s","jobs_timeout":"%s","harness_exit":%d}\n' \
  "$listed" "$runnable" "$passed" "$failed" "$timed" "$skipped" \
  "$esc_testee" "$BASH53_USERLAND" "$esc_tree" "$esc_goversion" \
  "$BASH53_TIMEOUT" "$BASH53_JOBS_TIMEOUT" "$rc" \
  > windows-bash53-gate-counts.json
{
  echo "## GNU Bash 5.3 fixtures on Windows — release gate"
  echo
  echo "**$passed / $runnable** runnable fixtures pass ($testee)."
  echo
  echo "| listed | runnable | passed | failed | timed out | skipped |"
  echo "|---:|---:|---:|---:|---:|---:|"
  echo "| $listed | $runnable | $passed | $failed | $timed | $skipped |"
  echo
  echo '```'
  echo "$results"
  echo '```'
  echo
  echo "Testee: \`$testee_path\`; userland: \`$BASH53_USERLAND\`; fixture tree: \`$tree\`; per-fixture timeout $BASH53_TIMEOUT (jobs $BASH53_JOBS_TIMEOUT); harness exit $rc."
  echo
  echo "Non-passing fixtures:"
  echo
  echo '```'
  printf '%s' "$bad_list"
  echo '```'
} > windows-bash53-gate-summary.md

echo "gate: $results"
echo "gate: $passed/$runnable passed ($failed failed, $timed timed out, $skipped skipped of $listed listed; harness exit $rc)"
if [ -n "$bad_list" ]; then
  printf '%s' "$bad_list"
  echo "gate: SHORTFALL — non-passing fixtures above; evidence in $log" >&2
  exit 1
fi
echo "gate: GREEN — every runnable fixture passed."
exit 0
