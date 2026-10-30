#!/usr/bin/env bash
# Guard the native Go output contract without needing a Windows host.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
[[ $(GOOS=windows go env GOEXE) == .exe ]]
for tool in notepad form chart viewer whiteboard brainstorm; do
  case $tool in
    notepad|form|chart|viewer) launcher=$root/examples/desk/$tool/$tool.bsh; ignore=$root/examples/desk/$tool/.gitignore ;;
    *) launcher=$root/examples/$tool/$tool.bsh; ignore=$root/examples/$tool/server/.gitignore ;;
  esac
  if grep -q realpath "$launcher"; then echo "$tool uses realpath" >&2; exit 1; fi
  grep -Fq 'exe=$(go env GOEXE)' "$launcher"
  grep -Eq 'bin=.*\$exe' "$launcher"
  grep -Fq '[[ -f "$bin" ]] || stale=true' "$launcher"
  grep -Fq '[[ "$src" -nt "$bin" ]] && stale=true' "$launcher"
  grep -Fq 'if [[ $stale == true ]]; then' "$launcher"
  [[ $(grep -c 'go build -trimpath' "$launcher") == 1 ]]
  grep -Fq -- "-o \".desk-build/$tool\$exe\"" "$launcher"
  grep -Fq -- "mv -f \".desk-build/$tool\$exe\"" "$launcher"
  grep -Fq '|| [[ -f "$bin" ]]' "$launcher"
  grep -Fq '.desk-build/' "$ignore"
  grep -Fq 'exec "$bin"' "$launcher"
done
printf 'launcher paths: six relative GOEXE outputs, stale checks, and cache moves\n'
