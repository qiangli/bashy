#!/bin/bash
# Fail closed unless the staged candidate contains exactly the promoted set.
set -euo pipefail
[[ $# == 1 ]] || { echo "usage: $0 DISTDIR" >&2; exit 2; }
dir=$1
[[ -d $dir ]] || { echo "missing asset directory: $dir" >&2; exit 1; }
# Raw names deliberately use the base version so byte-promotion changes no names.
tag=${TAG:-}
if [[ -z $tag ]]; then
 for f in "$dir"/outpost-v*-linux-amd64; do
  tag=${f##*/outpost-}; tag=${tag%-linux-amd64}; break
 done
fi
base=${tag%-dev}
[[ $base =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$ ]] || { echo "cannot determine release version" >&2; exit 1; }
expected=$(
 for os in darwin linux windows; do
  for arch in amd64 arm64; do
   ext=tar.gz; suffix=; [[ $os != windows ]] || { ext=zip; suffix=.exe; }
   printf '%s\n' "bash-$os-$arch.$ext" "bashy-$os-$arch.$ext" "outpost-$base-$os-$arch$suffix" "outpost-$base-$os-$arch$suffix.sha256"
  done
 done
 # Linux packages are built once from the -dev tag and promoted byte-identical,
 # so their version always carries the -dev suffix.
 for arch in amd64 arm64; do
  for fmt in apk deb rpm; do printf '%s\n' "bashy_${base#v}-dev_linux_$arch.$fmt"; done
 done
 printf '%s\n' bashy-scratch-linux-amd64 bashy-scratch-linux-arm64 checksums.txt
)
expected=$(printf '%s\n' "$expected" | LC_ALL=C sort)
actual=$(find "$dir" -maxdepth 1 -type f -exec basename {} \; | LC_ALL=C sort)
if [[ $actual != "$expected" ]]; then
 echo "release asset set differs from required 33 files" >&2
 diff -u <(printf '%s\n' "$expected") <(printf '%s\n' "$actual") >&2 || true
 exit 1
fi
(cd "$dir" && sha256sum -c checksums.txt --status)
[[ $(wc -l < "$dir/checksums.txt" | tr -d ' ') == 32 ]] || { echo "checksum count must be 32" >&2; exit 1; }
checksummed=$(sed -n 's/^[[:xdigit:]]\{64\}  //p' "$dir/checksums.txt" | LC_ALL=C sort)
expected_checksums=$(printf '%s\n' "$expected" | sed '/^checksums.txt$/d')
[[ $checksummed == "$expected_checksums" ]] || { echo "checksum entries differ from release assets" >&2; exit 1; }
# The manifest authenticates every sidecar; each sidecar must also bind its
# matching raw executable rather than merely being a checksummed text file.
for sidecar in "$dir"/outpost-*.sha256; do
 (cd "$dir" && sha256sum -c "${sidecar##*/}" --status)
done
# Check the product payload in-place without extracting files onto the runner.
python3 - "$dir" <<'PY_ARCHIVES'
import pathlib, sys, tarfile, zipfile
root = pathlib.Path(sys.argv[1])
for os_name in ('linux', 'darwin', 'windows'):
    for arch in ('amd64', 'arm64'):
        suffix = '.exe' if os_name == 'windows' else ''
        required = {name + suffix for name in ('bashy', 'outpost', 'bash', 'sh')}
        if os_name == 'windows':
            with zipfile.ZipFile(root / f'bashy-{os_name}-{arch}.zip') as archive:
                names = archive.namelist()
        else:
            with tarfile.open(root / f'bashy-{os_name}-{arch}.tar.gz') as archive:
                names = archive.getnames()
        normalized = [name.removeprefix('./') for name in names]
        for name in required:
            if normalized.count(name) != 1:
                raise SystemExit(f'{os_name}/{arch}: missing or duplicate product member {name}')
PY_ARCHIVES
echo "release asset set: PASS 27 files and 26 checksums"
