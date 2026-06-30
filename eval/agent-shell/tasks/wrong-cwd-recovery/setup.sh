#!/bin/bash
set -euo pipefail

work=$1
mkdir -p "$work/scripts" "$work/data" "$work/nested/deep" "$work/out"

cat >"$work/data/items.tsv" <<'DATA'
alpha	3
beta	5
gamma	8
DATA

cat >"$work/scripts/report.sh" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
mkdir -p out
awk -F '\t' '{sum += $2} END {print "TOTAL=" sum}' data/items.tsv > out/report.txt
SCRIPT
chmod +x "$work/scripts/report.sh"

cat >"$work/README.md" <<'README'
# Wrong CWD Recovery Task

The report command is `scripts/report.sh`. It must be run from the repository
root because it reads `data/items.tsv` and writes `out/report.txt`.
README
