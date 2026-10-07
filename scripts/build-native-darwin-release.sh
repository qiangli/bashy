#!/bin/bash
# Build one audited, runtime-probed native macOS Bashy archive for a -dev tag.
set -euo pipefail
if [[ $# != 3 ]]; then echo "usage: $0 vX.Y.Z-dev amd64|arm64 OUTDIR" >&2; exit 2; fi
tag=$1; arch=$2; outdir=$3
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+-dev$ ]] || { echo "invalid release tag: $tag" >&2; exit 2; }
[[ $arch == amd64 || $arch == arm64 ]] || { echo "invalid arch: $arch" >&2; exit 2; }
[[ $(go env GOOS) == darwin && $(go env GOARCH) == "$arch" ]] || { echo "native darwin/$arch builder required" >&2; exit 1; }
[[ $(go env CGO_ENABLED) == 1 ]] || { echo "native Darwin release requires cgo" >&2; exit 1; }
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
mkdir -p "$outdir"
outdir=$(cd "$outdir" && pwd)
name="bashy-darwin-$arch.tar.gz"
[[ ! -e "$outdir/$name" ]] || { echo "refusing to overwrite $outdir/$name" >&2; exit 1; }
stage=$(mktemp -d "${TMPDIR:-/tmp}/bashy-darwin-release.XXXXXX")
trap 'rm -rf "$stage"' EXIT
shell_commit=$(sed -n 's/^sh=//p' .sibling-pins)
[[ -n $shell_commit ]] && git -C ../sh cat-file -e "$shell_commit^{commit}"
shell_time=$(git -C ../sh show -s --format=%cI "$shell_commit")
base=${tag#v}; base=${base%-dev}
CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" scripts/go-product.sh build -trimpath -ldflags "-w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-$base' -X 'github.com/qiangli/bashsharp/transpile.ShellRuntimeCommit=$shell_commit' -X 'github.com/qiangli/bashsharp/transpile.ShellRuntimeCommitTime=$shell_time'" -o "$stage/bashy" ./cmd/bashy
./scripts/verify-bashy-signal-artifact.sh "$stage/bashy"
go run ./tools/bashysignalprobe "$stage/bashy"
./scripts/verify-meet-spa-release.sh "$stage/bashy" "darwin_$arch"
cp README.md LICENSE "$stage/"
COPYFILE_DISABLE=1 tar -czf "$stage/$name" -C "$stage" bashy README.md LICENSE
[[ $(tar -tzf "$stage/$name" | LC_ALL=C sort | tr '\n' ' ') == 'LICENSE README.md bashy ' ]] || { echo "bad Darwin archive contents" >&2; exit 1; }
mv "$stage/$name" "$outdir/$name"
shasum -a 256 "$outdir/$name"
