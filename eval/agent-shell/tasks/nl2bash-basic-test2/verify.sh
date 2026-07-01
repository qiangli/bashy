#!/bin/bash
set -euo pipefail

work=$1
cd "$work"
test -f test2.sh
chmod +x test2.sh
rm -f dir2/test.txt
./test2.sh
test -f dir1/test.txt
test -f dir2/test.txt
cmp -s dir1/test.txt dir2/test.txt
