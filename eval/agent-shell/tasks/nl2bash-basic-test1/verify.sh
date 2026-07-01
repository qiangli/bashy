#!/bin/bash
set -euo pipefail

work=$1
cd "$work"
test -f test1.sh
chmod +x test1.sh
rm -rf test
./test1.sh
test -d test
