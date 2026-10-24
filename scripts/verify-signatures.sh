#!/bin/bash
# Verify the published executable bytes according to the release-notes flag.
set -euo pipefail
[[ $# == 3 ]] || { echo "usage: $0 DIST darwin|windows true|false" >&2; exit 2; }
dir=$1 platform=$2 signed=$3
[[ $signed == true || $signed == false ]] || { echo 'missing or invalid signed flag' >&2; exit 2; }
if [[ $platform == windows ]]; then
 pwsh -NoProfile -File "$(dirname "$0")/verify-signatures.ps1" -Dist "$dir" -Signed "$signed"
 exit
fi
[[ $platform == darwin ]] || { echo "unknown platform: $platform" >&2; exit 2; }
if [[ $signed == false ]]; then echo 'UNSIGNED: Darwin release manifest declares signed=false'; exit 0; fi
work=$(mktemp -d "${TMPDIR:-/tmp}/bashy-signature-qa.XXXXXX")
trap 'rm -rf "$work"' EXIT
for arch in amd64 arm64; do
 mkdir "$work/$arch"
 tar -xzf "$dir/bashy-darwin-$arch.tar.gz" -C "$work/$arch"
 mkdir "$work/$arch-small"
 tar -xzf "$dir/bash-darwin-$arch.tar.gz" -C "$work/$arch-small"
 for name in bash sh; do cmp "$work/$arch/$name" "$work/$arch-small/$name"; done
 raw=("$dir"/outpost-v*-darwin-"$arch")
 [[ ${#raw[@]} == 1 && -f ${raw[0]} ]] || { echo "missing raw Darwin outpost for $arch" >&2; exit 1; }
 cmp "$work/$arch/outpost" "${raw[0]}"
 for name in bashy outpost bash sh; do
  binary="$work/$arch/$name"
  codesign --verify --strict --verbose=2 "$binary"
  spctl --assess --type execute --verbose=2 "$binary"
 done
done
echo 'Darwin Developer ID signatures and Gatekeeper assessments: PASS'
