#!/usr/bin/env bash
# Build the separate engine-enabled Linux host asset from this exact checkout.
# The ordinary GoReleaser release continues to produce the lean binary.
set -euo pipefail
[[ $# == 3 ]] || { echo 'usage: build-host-release.sh ARCH SOURCE_SHA OUTDIR' >&2; exit 2; }
arch=$1 source_sha=$2 outdir=$3
[[ $arch == amd64 || $arch == arm64 ]] || { echo 'unsupported architecture' >&2; exit 2; }
[[ $source_sha =~ ^[0-9a-f]{40}$ ]] || { echo 'source SHA must be a full commit ID' >&2; exit 2; }
[[ $(git rev-parse HEAD) == "$source_sha" ]] || { echo 'checkout does not match source SHA' >&2; exit 1; }
[[ $(go env GOARCH) == "$arch" ]] || { echo "native $arch runner required" >&2; exit 1; }
[[ $(go env GOOS) == linux ]] || { echo 'Linux runner required' >&2; exit 1; }
[[ ${BASHY_ENGINES:-} == 1 ]] || { echo 'BASHY_ENGINES=1 required' >&2; exit 1; }
[[ ${CGO_ENABLED:-} == 1 ]] || { echo 'CGO_ENABLED=1 required' >&2; exit 1; }
mkdir -p "$outdir"
binary="$outdir/bashy-host-linux-$arch"
scripts/build-meet-spa.sh check
# The host serves the embedded Ollama engine. Keep Podman external: its linked
# engine brings MPL-licensed dependencies into the release binary.
scripts/go-product.sh build -trimpath -tags bashy_engines,embed_runner,bashy_host \
  -ldflags "-w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-host-$source_sha'" \
  -o "$binary" ./cmd/bashy
go version -m "$binary" > "$binary.go-version.txt"
grep -Fq 'bashy_engines' "$binary.go-version.txt"
grep -Fq 'embed_runner' "$binary.go-version.txt"
grep -Fq 'bashy_host' "$binary.go-version.txt"
scripts/verify-bashy-signal-artifact.sh "$binary"
scripts/generate-release-sbom.sh "$binary" linux "$arch" "$outdir"
sha=$(sha256sum "$binary")
sha=${sha%% *}
python3 - "$outdir/host-asset-manifest-linux-$arch.json" "$source_sha" "$arch" "$sha" <<'PY'
import json, sys
path, source, arch, digest = sys.argv[1:]
with open(path, "w") as f:
    json.dump({"schema": "bashy-host-asset-v1", "source_sha": source,
               "profile": "BASHY_ENGINES=1", "cgo_enabled": True,
               "goos": "linux", "goarch": arch,
               "go_tags": ["bashy_engines", "embed_runner", "bashy_host"],
               "asset": f"bashy-host-linux-{arch}", "sha256": digest},
              f, indent=2, sort_keys=True)
    f.write("\n")
PY
printf 'HOST-ASSET-BUILT %s %s\n' "$arch" "$sha"
