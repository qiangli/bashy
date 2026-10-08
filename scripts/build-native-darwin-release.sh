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
shell_commit=$(sed -n 's|^replace mvdan.cc/sh/v3 => github.com/qiangli/sh/v3 ||p' go.mod)
[[ -n $shell_commit ]] || { echo "missing sh fork replace in go.mod" >&2; exit 1; }
shell_time=
base=${tag#v}; base=${base%-dev}
CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" scripts/go-product.sh build -trimpath -ldflags "-w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-$tag' -X 'github.com/qiangli/bashsharp/transpile.ShellRuntimeCommit=$shell_commit' -X 'github.com/qiangli/bashsharp/transpile.ShellRuntimeCommitTime=$shell_time'" -o "$stage/bashy" ./cmd/bashy
./scripts/verify-bashy-signal-artifact.sh "$stage/bashy"
go run ./tools/bashysignalprobe "$stage/bashy"
./scripts/verify-meet-spa-release.sh "$stage/bashy" "darwin_$arch"
./scripts/generate-release-sbom.sh "$stage/bashy" darwin "$arch" "${RUNNER_TEMP:-/tmp}/darwin-sbom"
# ../outpost is the go.mod-pinned outpost module (scripts/resolve-pinned-siblings.sh).
outpost_commit=${OUTPOST_COMMIT:-}
[[ $outpost_commit =~ ^[0-9a-f]{40}$ ]] || { echo "missing OUTPOST_COMMIT (run scripts/resolve-pinned-siblings.sh)" >&2; exit 1; }
for shell in bash sh; do
 CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" scripts/go-product.sh build -trimpath -ldflags "-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-$tag'" -o "$stage/$shell" "./cmd/$shell"
done
(cd ../outpost && CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -tags fb_archives -ldflags "-s -w -X github.com/qiangli/outpost/internal/agent.releaseTag=$tag -X github.com/qiangli/outpost/internal/agent.ldCommit=$outpost_commit -X github.com/qiangli/outpost/internal/agent.ldDirty=false" -o "$stage/outpost" ./cmd/outpost)
cp README.md LICENSE "$stage/"
cp ../outpost/LICENSE "$stage/LICENSE.outpost"
COPYFILE_DISABLE=1 tar -czf "$stage/$name" -C "$stage" bashy outpost bash sh README.md LICENSE LICENSE.outpost
[[ $(tar -tzf "$stage/$name" | LC_ALL=C sort | tr '\n' ' ') == 'LICENSE LICENSE.outpost README.md bash bashy outpost sh ' ]] || { echo "bad Darwin archive contents" >&2; exit 1; }
mv "$stage/$name" "$outdir/$name"
COPYFILE_DISABLE=1 tar -czf "$outdir/bash-darwin-$arch.tar.gz" -C "$stage" bash sh README.md LICENSE
cp "$stage/outpost" "$outdir/outpost-v$base-darwin-$arch"
shasum -a 256 "$outdir/$name"
