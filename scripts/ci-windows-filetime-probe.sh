#!/usr/bin/env bash
# Measure the native Windows file timestamps used by test -N in the same
# prepared Bashy root/userland context as the Windows fixture runner.
#
# This is intentionally a probe, not a conformance test: it records what the
# hosted runner's filesystem does and never changes shell/stat behaviour.
set -euo pipefail
cd "$(dirname "$0")/.."

command -v pwsh >/dev/null || {
  echo 'windows-filetime-probe: pwsh is required on windows-latest' >&2
  exit 2
}
log=windows-filetime-probe.log
: > "$log"

{
  echo "probe: runner_os=$RUNNER_OS"
  echo "probe: shell=bin/bash.exe via bash53suite under Git Bash"
  echo "probe: pwd=$(pwd)"
  echo "probe: RUNNER_TEMP=${RUNNER_TEMP:-unset}"
  case "$(go env GOOS)" in windows) ;; *) echo 'probe: not a Windows build host' >&2; exit 2 ;; esac
  mkdir -p bin
  CGO_ENABLED=0 go build -o bin/bash.exe ./cmd/bash
  go build -o bin/bash53suite.exe ./tools/bash53suite
  (cd ../yoke && CGO_ENABLED=0 go build -o "$OLDPWD/bin/yoke.exe" ./cmd/yoke)
  tree=$(go run ./tools/bash53fixtures -root .)
  [ -d "$tree/tests" ] || { echo "probe: no fixture tree at $tree" >&2; exit 2; }
  echo "probe: userland=bin/yoke.exe"
  ./bin/bash53suite.exe -tests-dir "$tree/tests" -bash bin/bash.exe -userland bin/yoke.exe -filetime-probe
} 2>&1 | tee "$log"

echo 'probe: artifact=windows-filetime-probe.log'
