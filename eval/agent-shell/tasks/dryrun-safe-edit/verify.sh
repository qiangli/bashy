#!/bin/bash
set -euo pipefail

work=$1
cd "$work"
./scripts/cleanup.sh
test -e fixtures/keep.txt
test ! -e build/tmp/generated.txt
