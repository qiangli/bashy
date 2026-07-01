#!/bin/bash
set -euo pipefail

work=$1
mkdir -p "$work"
cat >"$work/README.md" <<'README'
# IBM NL2Bash Basic Test 3

Create `test3.sh` for the prompt in the benchmark instructions.
README
