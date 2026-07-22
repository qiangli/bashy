#!/usr/bin/env bash
# publish-otel-blob.sh — build the otel stack binary for EVERY platform and
# upload it to a bashy release as otel-<goos>-<goarch>.gz, which binmgr fetches
# on demand (internal/agentos/obs_stub.go -> binmgr.ProvisionManaged).
#
# WHY THIS IS SEPARATE FROM publish-engine-blobs.sh
#
# That script is per-runner and macOS-only because podman needs a native cgo
# toolchain and its own source bootstrap, so its matrix is one runner per
# platform. otel has none of those constraints: it is our own code
# (coreutils/external/otel), pure Go, CGO_ENABLED=0 — verified building for
# darwin/{arm64,amd64}, linux/{amd64,arm64} and windows/amd64 from one host.
#
# So it cross-compiles the whole matrix in a single job. Folding it into the
# engine script would have inherited podman's runner matrix and its podman
# source bootstrap for a binary that needs neither, and would have left
# windows and linux with no otel blob at all — the engine matrix does not
# cover them.
#
# WHAT IS *NOT* IN THE BLOB: VictoriaLogs/Metrics/Traces. The stack fetches
# those upstream release binaries at runtime (external/otel/stack/execstore.go)
# and runs them as subprocesses. This blob is the orchestrator only, which is
# why it is small and why its license story is just ours.
#
# Usage: scripts/publish-otel-blob.sh [RELEASE_TAG]   (default: latest release)
set -eu

HERE=$(cd "$(dirname "$0")/.." && pwd)
CU="${COREUTILS_DIR:-$HERE/../coreutils}"
REPO="${BASHY_REPO:-qiangli/bashy}"
TAG="${1:-}"
SRC="$CU/external/otel"

if [ ! -d "$SRC" ]; then
  echo "publish-otel-blob: no otel source at $SRC (set COREUTILS_DIR)" >&2
  exit 1
fi

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

# The platforms binmgr can ask for. Keep in step with binmgr.Platform().
PLATFORMS="darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64"

for p in $PLATFORMS; do
  goos="${p%/*}"; goarch="${p#*/}"
  out="$STAGE/otel"
  [ "$goos" = windows ] && out="$STAGE/otel.exe"
  echo "publish-otel-blob: building $goos/$goarch"
  ( cd "$SRC" && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags="-s -w" -o "$out" ./cmd/otel )
  # Asset name must match binmgr.fetchReleaseGz: <name>-<goos>-<goarch>.gz,
  # with no .exe in the asset name — the .exe is added on extract, by
  # binaryName(), not carried in the blob name.
  gzip -c "$out" > "$STAGE/otel-$goos-$goarch.gz"
  rm -f "$out"
  # Integrity check the stream we are about to publish, rather than trusting
  # that a build that exited 0 produced a valid one.
  gzip -t "$STAGE/otel-$goos-$goarch.gz"
done

cat > "$STAGE/otel-blob-NOTICE.txt" <<'EOF'
bashy otel blob — the embedded observability stack orchestrator
(coreutils/external/otel), built from source in this project.

It is fetched at runtime and exec'd as a separate process; it is not linked
into bashy. The storage/query components it supervises are NOT in this blob —
VictoriaLogs, VictoriaMetrics and VictoriaTraces (all Apache-2.0) are fetched
from their upstream releases on first use and run as subprocesses.
EOF

if [ -z "$TAG" ]; then
  TAG=$(gh release view --repo "$REPO" --json tagName --jq .tagName)
fi
echo "publish-otel-blob: uploading otel blobs to $REPO@$TAG"
gh release upload "$TAG" "$STAGE"/otel-*.gz "$STAGE/otel-blob-NOTICE.txt" --repo "$REPO" --clobber
echo "publish-otel-blob: done (${PLATFORMS} -> $TAG)"
