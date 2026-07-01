#!/bin/bash
set -euo pipefail

work=$1
cd "$work"
test -f test3.sh
chmod +x test3.sh
rm -f test.json
./test3.sh
test -f test.json
grep -Eq '"name"[[:space:]]*:[[:space:]]*"test"' test.json
