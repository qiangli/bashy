#!/usr/bin/env bash
# publish-engine-blobs.sh — build bashy's permissive engine blobs FROM SOURCE for
# the CURRENT platform and upload them to a bashy release as
# <name>-<goos>-<goarch>.gz, which the lean binary fetches on demand
# (internal/agentos/engines_stub.go, Tier 2).
#
# Run it on each target machine to populate the matrix (darwin on a Mac,
# linux on a Linux box); .github/workflows/engine-blobs.yml calls it on GitHub
# runners. Idempotent: reuses an already-built embed blob when present.
#
# Licensing (docs/licensing-supply-chain-policy.md): podman/gvproxy/vfkit are
# Apache-2.0, built from source — never linked into bashy, fetched + exec'd as
# separate processes. An attribution NOTICE is uploaded alongside.
#
# Usage: scripts/publish-engine-blobs.sh [RELEASE_TAG]   (default: latest release)
set -eu

HERE=$(cd "$(dirname "$0")/.." && pwd)
CU="${COREUTILS_DIR:-$HERE/../coreutils}"
REPO="${BASHY_REPO:-qiangli/bashy}"
TAG="${1:-}"
GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)
EMB="$CU/external/podman/engine"

case "$GOOS" in
  darwin) ENGINES="podman vfkit gvproxy" ;;   # mac needs the VM triad
  linux)  ENGINES="podman gvproxy" ;;         # vfkit is macOS-only
  *) echo "publish-engine-blobs: no local engine blobs for $GOOS (podman is remote/mesh here)"; exit 0 ;;
esac

blob_src() {
  case "$1" in
    podman)  echo "$EMB/podman_embed/podman.gz" ;;
    vfkit)   echo "$EMB/vfkit_embed/vfkit.gz" ;;
    gvproxy) echo "$EMB/gvproxy_embed/gvproxy.gz" ;;
  esac
}

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

for e in $ENGINES; do
  src=$(blob_src "$e")
  if [ ! -f "$src" ]; then
    echo "publish-engine-blobs: building $e from source…"
    bash "$CU/scripts/embed-$e.sh" || echo "  (embed-$e.sh returned nonzero)"
  fi
  if [ -f "$src" ]; then
    # sanity: the blob must be a valid gzip stream (integrity test, no extract)
    if gunzip -t "$src" 2>/dev/null; then
      cp "$src" "$STAGE/$e-$GOOS-$GOARCH.gz"
      echo "  staged $e-$GOOS-$GOARCH.gz ($(ls -lh "$src" | awk '{print $5}'))"
    else
      echo "  WARN: $e blob is empty/corrupt — skipping"
    fi
  else
    echo "  WARN: $e blob not produced — skipping"
  fi
done

if ! ls "$STAGE"/*.gz >/dev/null 2>&1; then
  echo "publish-engine-blobs: nothing staged for $GOOS-$GOARCH" >&2
  exit 1
fi

cat > "$STAGE/engine-blobs-NOTICE-$GOOS-$GOARCH.txt" <<EOF
bashy engine blobs ($GOOS/$GOARCH) — built from permissive source, redistributed
under the upstream licenses. NOT linked into bashy; fetched at runtime and run as
separate processes (docs/licensing-supply-chain-policy.md).

  podman    Apache-2.0   github.com/containers/podman
  gvproxy   Apache-2.0   github.com/containers/gvisor-tap-vsock
  vfkit     Apache-2.0   github.com/crc-org/vfkit
EOF

if [ -z "$TAG" ]; then
  TAG=$(gh release view --repo "$REPO" --json tagName --jq .tagName)
fi
echo "publish-engine-blobs: uploading $GOOS-$GOARCH blobs to $REPO@$TAG"
gh release upload "$TAG" "$STAGE"/*.gz "$STAGE"/engine-blobs-NOTICE-*.txt --repo "$REPO" --clobber
echo "publish-engine-blobs: done ($GOOS-$GOARCH -> $TAG)"
