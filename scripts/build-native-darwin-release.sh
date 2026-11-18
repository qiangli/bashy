#!/bin/bash
# Build one audited, runtime-probed native macOS Bashy archive for a -dev tag.
set -euo pipefail
if [[ $# != 3 ]]; then echo "usage: $0 vX.Y.Z-dev amd64|arm64 OUTDIR" >&2; exit 2; fi
tag=$1; arch=$2; outdir=$3
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?-dev$ ]] || { echo "invalid release tag: $tag" >&2; exit 2; }
[[ $arch == amd64 || $arch == arm64 ]] || { echo "invalid arch: $arch" >&2; exit 2; }
[[ $(go env GOOS) == darwin && $(go env GOARCH) == "$arch" ]] || { echo "native darwin/$arch builder required" >&2; exit 1; }
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
mkdir -p "$outdir"
outdir=$(cd "$outdir" && pwd)
name="bashy-darwin-$arch.tar.gz"
[[ ! -e "$outdir/$name" ]] || { echo "refusing to overwrite $outdir/$name" >&2; exit 1; }
stage=$(mktemp -d "${TMPDIR:-/tmp}/bashy-darwin-release.XXXXXX")
trap 'rm -rf "$stage"' EXIT
shell_commit=$(sed -n -E 's|^[[:space:]]*(require[[:space:]]+)?github[.]com/qiangli/bash[[:space:]]+([^[:space:]]+).*|\2|p' go.mod)
[[ -n $shell_commit ]] || { echo "missing sh fork replace in go.mod" >&2; exit 1; }
shell_time=
source_commit=$(git rev-parse HEAD)
[[ $source_commit =~ ^[0-9a-f]{40}$ ]] || { echo "invalid source commit: $source_commit" >&2; exit 1; }
base=${tag#v}; base=${base%-dev}
# The product binary embeds the signed Ollama runner (scripts/fetch-signed-runner.sh
# in release.yml). Refuse to ship a runner-less binary with an explicit error
# rather than relying on the go:embed failure alone.
build_yoke_dir=$(GOWORK=off go list -m -f '{{.Dir}}' github.com/qiangli/yoke)
[[ -f "$build_yoke_dir/external/ollama/runner_embed/ycode-runner.gz" && -f "$build_yoke_dir/external/ollama/runner_embed/ycode-runner.sigstore.json" ]] \
 || { echo "missing verified runner: run scripts/fetch-signed-runner.sh darwin $arch first" >&2; exit 1; }
CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" scripts/go-product.sh build -trimpath -tags embed_runner -ldflags "-w -X 'github.com/bashsharp/bashsharp/cli.bashVersion=5.3.0(1)-bashy-$tag' -X 'github.com/bashsharp/bashsharp/cli.buildID=$source_commit' -X 'github.com/bashsharp/bashsharp/transpile.ShellRuntimeCommit=$shell_commit' -X 'github.com/bashsharp/bashsharp/transpile.ShellRuntimeCommitTime=$shell_time'" -o "$stage/bashy" ./cmd/bashy
./scripts/verify-bashy-signal-artifact.sh "$stage/bashy"
scripts/go-tool.sh bashysignalprobe "$stage/bashy"
./scripts/verify-meet-spa-release.sh "$stage/bashy" "darwin_$arch"
./scripts/generate-release-sbom.sh "$stage/bashy" darwin "$arch" "${RUNNER_TEMP:-/tmp}/darwin-sbom"
# ../outpost is the go.mod-pinned outpost module (scripts/resolve-pinned-siblings.sh).
outpost_commit=${OUTPOST_COMMIT:-}
[[ $outpost_commit =~ ^[0-9a-f]{40}$ ]] || { echo "missing OUTPOST_COMMIT (run scripts/resolve-pinned-siblings.sh)" >&2; exit 1; }
# Source-separated shell products use their own command module and product driver.
shell_root=${BASH_SOURCE_ROOT:-$root/../bash}
shell_source_commit=$(git -C "$shell_root" rev-parse HEAD)
[[ $shell_source_commit =~ ^[0-9a-f]{40}$ ]] || { echo "invalid standalone Bash commit" >&2; exit 1; }
[[ -z $(git -C "$shell_root" status --porcelain) ]] || { echo "standalone Bash source is dirty" >&2; exit 1; }
for shell in bash sh; do
 (cd "$shell_root/cmd" && CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" ../scripts/go-product.sh build -mod=readonly -trimpath -ldflags "-s -w -X github.com/bashsharp/bashsharp/cli.bashVersion=5.3.0(1)-bashy-$tag -X github.com/bashsharp/bashsharp/cli.buildID=$shell_source_commit" -o "$stage/$shell.real" "./$shell")
 cc -x c -std=c11 -O2 -Wall -Wextra -Werror -o "$stage/$shell" "$shell_root/native/siglaunch.c.in"
done
(cd ../outpost && CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -tags fb_archives -ldflags "-s -w -X github.com/qiangli/outpost/internal/agent.releaseTag=$tag -X github.com/qiangli/outpost/internal/agent.ldCommit=$outpost_commit -X github.com/qiangli/outpost/internal/agent.ldDirty=false" -o "$stage/outpost" ./cmd/outpost)
mkdir -p "$outdir/../signature-status"
./scripts/sign-darwin-release.sh "$stage" "$outdir/../signature-status/darwin-$arch.json"
cp README.md LICENSE "$stage/"
cp ../outpost/LICENSE "$stage/LICENSE.outpost"
COPYFILE_DISABLE=1 tar -czf "$stage/$name" -C "$stage" bashy outpost bash bash.real sh sh.real README.md LICENSE LICENSE.outpost
[[ $(tar -tzf "$stage/$name" | LC_ALL=C sort | tr '\n' ' ') == 'LICENSE LICENSE.outpost README.md bash bash.real bashy outpost sh sh.real ' ]] || { echo "bad Darwin archive contents" >&2; exit 1; }
mv "$stage/$name" "$outdir/$name"
COPYFILE_DISABLE=1 tar -czf "$outdir/bash-darwin-$arch.tar.gz" -C "$stage" bash bash.real sh sh.real README.md LICENSE
cp "$stage/outpost" "$outdir/outpost-v$base-darwin-$arch"
shasum -a 256 "$outdir/$name"
