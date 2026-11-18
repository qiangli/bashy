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
   [[ $os != darwin || $arch != amd64 ]] || continue
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
 # The otel stack orchestrator blobs (binmgr fetches otel-<goos>-<goarch>.gz
 # from the latest release) plus their NOTICE. Built by the release
 # workflow's otel job via scripts/publish-otel-blob.sh — keep this list in
 # step with that script's PLATFORMS. A candidate without them fails here, so
 # an otel-less release can never publish or promote.
 for p in darwin/arm64 linux/amd64 linux/arm64 windows/amd64; do
  printf '%s\n' "otel-${p%/*}-${p#*/}.gz"
 done
 printf '%s\n' otel-blob-NOTICE.txt
)
expected=$(printf '%s\n' "$expected" | LC_ALL=C sort)
actual=$(find "$dir" -maxdepth 1 -type f -exec basename {} \; | LC_ALL=C sort)
if [[ $actual != "$expected" ]]; then
 echo "release asset set differs from required files" >&2
 diff -u <(printf '%s\n' "$expected") <(printf '%s\n' "$actual") >&2 || true
 exit 1
fi
(cd "$dir" && sha256sum -c checksums.txt --status)
expected_count=$(printf '%s\n' "$expected" | wc -l | tr -d ' ')
[[ $(wc -l < "$dir/checksums.txt" | tr -d ' ') == $((expected_count - 1)) ]] || { echo "checksum count differs from required assets" >&2; exit 1; }
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
        if os_name == 'darwin' and arch == 'amd64':
            continue
        suffix = '.exe' if os_name == 'windows' else ''
        for family, members in [('bashy', ('bashy', 'outpost', 'bash', 'sh')), ('bash', ('bash', 'sh'))]:
            extension = 'zip' if os_name == 'windows' else 'tar.gz'
            path = root / f'{family}-{os_name}-{arch}.{extension}'
            if os_name == 'windows':
                with zipfile.ZipFile(path) as archive:
                    names = archive.namelist()
                    data = {name.removeprefix('./'): archive.read(name) for name in names if not name.endswith('/')}
            else:
                with tarfile.open(path) as archive:
                    names = archive.getnames()
                    data = {m.name.removeprefix('./'): archive.extractfile(m).read() for m in archive.getmembers() if m.isfile() or m.issym()}
            normalized = [name.removeprefix('./') for name in names]
            for name in members:
                if normalized.count(name + suffix) != 1:
                    raise SystemExit(f'{family} {os_name}/{arch}: missing or duplicate product member {name + suffix}')
            # The canonical native C launcher loads a sibling .real payload.
            # Cross-target canonical builds remain plain Go executables; do
            # not invent a uniform native-pair layout for those artifacts.
            for name in ('bash', 'sh'):
                body = data[name + suffix]
                native = body.startswith((b'\x7fELF', b'\xcf\xfa\xed\xfe', b'\xfe\xed\xfa\xcf'))
                if native and b'\xff Go buildinf:' not in body:
                    if normalized.count(name + '.real') != 1:
                        raise SystemExit(f'{family} {os_name}/{arch}: native launcher is missing {name}.real')

PY_ARCHIVES
echo "release asset set: PASS $expected_count files and $((expected_count - 1)) checksums"
