#!/bin/bash
# Fail closed unless the staged candidate contains exactly the promoted set.
set -euo pipefail
[[ $# == 1 ]] || { echo "usage: $0 DISTDIR" >&2; exit 2; }
dir=$1
[[ -d $dir ]] || { echo "missing asset directory: $dir" >&2; exit 1; }
expected=$(cat <<'NAMES'
bash-darwin-amd64.tar.gz
bash-darwin-arm64.tar.gz
bash-linux-amd64.tar.gz
bash-linux-arm64.tar.gz
bash-windows-amd64.zip
bash-windows-arm64.zip
bashy-darwin-amd64.tar.gz
bashy-darwin-arm64.tar.gz
bashy-linux-amd64.tar.gz
bashy-linux-arm64.tar.gz
bashy-scratch-linux-amd64
bashy-scratch-linux-arm64
bashy-windows-amd64.zip
bashy-windows-arm64.zip
checksums.txt
NAMES
)
actual=$(find "$dir" -maxdepth 1 -type f -exec basename {} \; | LC_ALL=C sort)
if [[ $actual != "$expected" ]]; then
 echo "release asset set differs from required 15 files" >&2
 diff -u <(printf '%s\n' "$expected") <(printf '%s\n' "$actual") >&2 || true
 exit 1
fi
(cd "$dir" && sha256sum -c checksums.txt --status)
[[ $(wc -l < "$dir/checksums.txt" | tr -d ' ') == 14 ]] || { echo "checksum count must be 14" >&2; exit 1; }
checksummed=$(sed -n 's/^[[:xdigit:]]\{64\}  //p' "$dir/checksums.txt" | LC_ALL=C sort)
expected_checksums=$(printf '%s\n' "$expected" | sed '/^checksums.txt$/d')
[[ $checksummed == "$expected_checksums" ]] || { echo "checksum entries differ from release assets" >&2; exit 1; }
echo "release asset set: PASS 15 files and 14 checksums"
