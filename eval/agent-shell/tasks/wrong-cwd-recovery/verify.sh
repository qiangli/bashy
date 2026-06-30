#!/bin/bash
set -euo pipefail

work=$1
cd "$work"
test -s out/report.txt
grep -q '^TOTAL=16$' out/report.txt
