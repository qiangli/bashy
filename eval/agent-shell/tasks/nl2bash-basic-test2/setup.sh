#!/bin/bash
set -euo pipefail

work=$1
mkdir -p "$work/dir1" "$work/dir2"
printf 'fixture\n' >"$work/dir1/test.txt"
cat >"$work/README.md" <<'README'
# IBM NL2Bash Basic Test 2

Create `test2.sh` for the prompt in the benchmark instructions.
README
