#!/bin/bash
set -euo pipefail

work=$1
cd "$work"
cmp -s scripts/make-summary.sh .benchmark/make-summary.sh.orig
test -f reports/summary.txt
grep -qx 'TOTAL=47' reports/summary.txt
