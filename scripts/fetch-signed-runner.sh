#!/bin/sh
# Fetch the signed Ollama runner for one release target and stage it for
# `-tags embed_runner` builds.
#
# Usage: ./scripts/fetch-signed-runner.sh OS ARCH
#    e.g. ./scripts/fetch-signed-runner.sh darwin arm64
#
# Signed targets (the only ones the pinned release publishes):
#   linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
# Other build targets have no runner to embed; the release hook skips them.
#
# For signed targets, every fetch and verification step fails closed:
#   1. Downloads runner-OS-ARCH.gz, its Sigstore bundle
#      (runner-OS-ARCH.gz.sigstore.json) and the raw executable's bundle
#      (runner-OS-ARCH.sigstore.json, windows: runner-windows-amd64.exe)
#      from the qiangli/ollama release RUNNER_TAG.
#   2. Verifies the .gz against its bundle with the pinned cosign (the same
#      verifier binmgr uses: yoke's cosign pin + embedded trusted root,
#      `cosign verify-blob --bundle ... --certificate-identity ... \
#        --certificate-oidc-issuer ... --trusted-root ... --offline`),
#      requiring EXACTLY RUNNER_IDENTITY / RUNNER_ISSUER.
#   3. Decompresses the .gz and requires sha256(executable bytes) to equal
#      the value pinned in yoke
#      (external/ollama/runner_embed/extract.go releaseSHA256) — the same pin
#      EnsureRunner enforces at runtime. An empty or missing pin refuses.
#   4. Verifies the executable's bundle over those exact decompressed bytes
#      (this is the bundle embedded for runtime verification).
#   5. Writes ycode-runner.gz + ycode-runner.sigstore.json into yoke's
#      checkout and the pinned Go module used by release builds.
#
# The embedded bundle MUST be the signature over the final executable bytes:
# extract.go runs binmgr.VerifySignedBlob over the installed executable, which
# requires bundle-digest == pin == executable sha256. Embedding the .gz's
# bundle instead would verify here but fail every launch.
#
# Env overrides (defaults suit CI and local runs):
#   YOKE_DIR          yoke checkout (default: <repo-root>/../yoke)
#   SIGNED_RUNNER_CACHE  verified-download cache dir
#                        (default: ${TMPDIR:-/tmp}/bashy-signed-runner)
# Test seams (never set in CI/release):
#   COSIGN_BIN + ALLOW_UNPINNED_COSIGN=1
#                       use this cosign instead of the pinned download
#   RUNNER_BASE_URL     release download base URL (tests serve fixtures)
set -eu

RUNNER_TAG="bashy-v0.21.2-15"
RUNNER_REPO="qiangli/ollama"
RUNNER_IDENTITY="https://github.com/qiangli/ollama/.github/workflows/bashy-build.yml@refs/tags/${RUNNER_TAG}"
RUNNER_ISSUER="https://token.actions.githubusercontent.com"

die() { echo "fetch-signed-runner: $*" >&2; exit 1; }

if [ "$#" -eq 2 ]; then
  os=$1 arch=$2
elif [ "$#" -eq 1 ]; then
  case $1 in
    */*) os=${1%/*}; arch=${1#*/} ;;
    *_*) os=${1%_*}; arch=${1#*_} ;;
    *) die "usage: $0 OS ARCH (e.g. $0 darwin arm64)" ;;
  esac
else
  die "usage: $0 OS ARCH (e.g. $0 darwin arm64)"
fi

# Only these targets have signed assets in the pinned release. GoReleaser also
# builds windows/arm64, so a target without an asset must let that build proceed.
case ${os}/${arch} in
  linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64) ;;
  *) echo "fetch-signed-runner: skipping ${os}/${arch}: no signed runner in ${RUNNER_TAG}"; exit 0 ;;
esac

script_dir=$(dirname -- "$0")
root=$(cd -- "${script_dir}/.." && pwd)
yoke_dir=${YOKE_DIR:-$root/../yoke}
embed_dir=$yoke_dir/external/ollama/runner_embed
extract_go=$embed_dir/extract.go
[ -f "$extract_go" ] || die "yoke checkout missing: $extract_go (set YOKE_DIR)"

# Pinned executable sha256 from yoke — the same pin EnsureRunner enforces.
pin=$(sed -n "s/^.*\"${os}\/${arch}\":[[:space:]]*\"\([0-9a-f]*\)\".*/\1/p" "$extract_go" | head -n 1)
case $pin in
  *[!0-9a-f]*|"") die "no pinned sha256 for ${os}/${arch} in $extract_go" ;;
esac
[ "${#pin}" -eq 64 ] || die "no pinned sha256 for ${os}/${arch} in $extract_go (empty or malformed pin)"

# The script and yoke must name the same release: refuse a yoke checkout
# whose release metadata disagrees with this signed release.
yoke_identity=$(sed -n 's/^.*releaseIdentity[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$extract_go" | head -n 1)
yoke_version=$(sed -n 's/^.*releaseVersion[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$extract_go" | head -n 1)
[ -n "$yoke_identity" ] || die "yoke releaseIdentity is empty in $extract_go"
[ "$yoke_version" = "$RUNNER_TAG" ] || die "yoke releaseVersion '$yoke_version' != $RUNNER_TAG"
[ "$yoke_identity" = "$RUNNER_IDENTITY" ] || die "yoke releaseIdentity '$yoke_identity' != $RUNNER_IDENTITY"

asset="runner-${os}-${arch}"
exe=$asset
if [ "$os" = "windows" ]; then exe="${asset}.exe"; fi
gz="${asset}.gz"
base_url=${RUNNER_BASE_URL:-https://github.com/${RUNNER_REPO}/releases/download/${RUNNER_TAG}}

cache_dir=${SIGNED_RUNNER_CACHE:-${TMPDIR:-/tmp}/bashy-signed-runner}
mkdir -p "$cache_dir" || die "cannot create cache dir $cache_dir"

# Pinned cosign: version + host-platform digest from yoke binmgr pins.
# The runner TARGET (os/arch args) selects the asset; the HOST selects the
# cosign binary (verification is platform-independent).
host_os=$(uname -s)
host_arch=$(uname -m)
case $host_os in
  Darwin) host_os=darwin ;;
  Linux) host_os=linux ;;
  *) die "unsupported cosign host: $(uname -s)" ;;
esac
case $host_arch in
  arm64|aarch64) host_arch=arm64 ;;
  x86_64|amd64) host_arch=amd64 ;;
  *) die "unsupported cosign arch: $(uname -m)" ;;
esac

resolve_cosign() {
  if [ -n "${COSIGN_BIN:-}" ]; then
    [ "${ALLOW_UNPINNED_COSIGN:-}" = "1" ] || die "COSIGN_BIN requires ALLOW_UNPINNED_COSIGN=1 (test seam only)"
    [ -x "$COSIGN_BIN" ] || die "COSIGN_BIN not executable: $COSIGN_BIN"
    echo "warning: using unpinned cosign $COSIGN_BIN (test only)" >&2
    printf '%s' "$COSIGN_BIN"
    return 0
  fi
  sigstore_go=$yoke_dir/pkg/binmgr/sigstore.go
  pins_go=$yoke_dir/pkg/binmgr/pins.go
  [ -f "$sigstore_go" ] && [ -f "$pins_go" ] || die "binmgr verifier pins missing under $yoke_dir"
  ver=$(sed -n 's/^.*CosignVersion[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$sigstore_go" | head -n 1)
  [ -n "$ver" ] || die "cannot parse CosignVersion from $sigstore_go"
  want=$(sed -n "s/^.*\"cosign@${ver}\/${host_os}\/${host_arch}\":[[:space:]]*\"\([0-9a-f]*\)\".*/\1/p" "$pins_go" | head -n 1)
  [ "${#want}" = 64 ] || die "no committed cosign pin for cosign@${ver}/${host_os}/${host_arch} in $pins_go"
  bin=$cache_dir/cosign-${host_os}-${host_arch}
  if [ "$host_os" = "windows" ]; then bin="${bin}.exe"; fi
  if [ -f "$bin" ]; then
    got=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$bin") \
      || die "cannot hash cached cosign"
    [ "$got" = "$want" ] || rm -f "$bin"
  fi
  if [ ! -f "$bin" ]; then
    name=cosign-${host_os}-${host_arch}
    if [ "$host_os" = "windows" ]; then name="${name}.exe"; fi
    curl -fsSL --retry 3 -o "$bin" "https://github.com/sigstore/cosign/releases/download/${ver}/${name}" \
      || die "cosign download failed"
    chmod +x "$bin" || die "cannot chmod cosign"
    got=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$bin") \
      || die "cannot hash downloaded cosign"
    [ "$got" = "$want" ] || { rm -f "$bin"; die "downloaded cosign does not match committed pin"; }
  fi
  printf '%s' "$bin"
}

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v python3 >/dev/null 2>&1 || die "python3 is required"

cosign=$(resolve_cosign)

# Pinned Sigstore trusted root from yoke (same root binmgr embeds).
root_json=$yoke_dir/pkg/binmgr/sigstore/trusted_root.json
[ -f "$root_json" ] || die "trusted root missing: $root_json"
want_root=$(sed -n 's/^.*trustedRootSHA256[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$yoke_dir/pkg/binmgr/sigstore.go" | head -n 1)
[ "${#want_root}" = 64 ] || die "cannot parse trustedRootSHA256"
got_root=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$root_json") \
  || die "cannot hash trusted root"
[ "$got_root" = "$want_root" ] || die "trusted root does not match committed pin"

stage=$(mktemp -d "${TMPDIR:-/tmp}/signed-runner.XXXXXX") || die "cannot create stage dir"
trap 'rm -rf "$stage"' EXIT HUP INT TERM

fetch() { # $1 = filename
  if [ -f "$cache_dir/${RUNNER_TAG}-$1" ]; then
    cp "$cache_dir/${RUNNER_TAG}-$1" "$stage/$1" || die "cannot stage cached $1"
  else
    curl -fsSL --retry 3 -o "$stage/$1" "${base_url}/$1" || die "download failed: $1"
    cp "$stage/$1" "$cache_dir/${RUNNER_TAG}-$1" || die "cannot cache $1"
  fi
}

fetch "$gz"
fetch "${gz}.sigstore.json"
fetch "${exe}.sigstore.json"

verify_blob() { # $1 = artifact path, $2 = bundle path
  "$cosign" verify-blob \
    --bundle "$2" \
    --certificate-identity "$RUNNER_IDENTITY" \
    --certificate-oidc-issuer "$RUNNER_ISSUER" \
    --trusted-root "$root_json" \
    --offline \
    "$1" >/dev/null 2>"$stage/cosign.err" \
    || { cat "$stage/cosign.err" >&2; die "sigstore verification failed for $1"; }
}

verify_blob "$stage/$gz" "$stage/${gz}.sigstore.json"

# Hash the FINAL executable bytes and require the yoke pin.
exe_sha=$(python3 - "$stage/$gz" "$stage/exe" <<'EOF'
import gzip, hashlib, sys
with gzip.open(sys.argv[1], 'rb') as f, open(sys.argv[2], 'wb') as out:
    h = hashlib.sha256()
    while True:
        chunk = f.read(1 << 20)
        if not chunk:
            break
        h.update(chunk)
        out.write(chunk)
print(h.hexdigest())
EOF
) || die "cannot decompress $gz"
[ -s "$stage/exe" ] || die "decompressed executable is empty"
[ "$exe_sha" = "$pin" ] || die "executable sha256 $exe_sha != pinned $pin"

# The embedded bundle must verify over those exact executable bytes.
verify_blob "$stage/exe" "$stage/${exe}.sigstore.json"

[ -d "$embed_dir" ] || die "embed dir missing: $embed_dir"
# CI checks out yoke for pin verification, but product builds load that same
# version from the Go module cache. Stage both source locations.
build_embed_dir=$embed_dir
if [ -z "${YOKE_DIR+x}" ]; then
  command -v go >/dev/null 2>&1 || die "go is required to locate the release module"
  build_yoke_dir=$(GOWORK=off go list -m -f '{{.Dir}}' github.com/qiangli/yoke) \
    || die "cannot locate pinned yoke module"
  build_embed_dir=$build_yoke_dir/external/ollama/runner_embed
  [ -d "$build_embed_dir" ] || die "build embed dir missing: $build_embed_dir"
  cmp -s "$extract_go" "$build_embed_dir/extract.go" \
    || die "checkout and build module have different runner pins"
fi
for destination in "$embed_dir" "$build_embed_dir"; do
  chmod u+w "$destination" || die "cannot make embed dir writable: $destination"
  cp "$stage/$gz" "$destination/ycode-runner.gz" || die "cannot install ycode-runner.gz"
  cp "$stage/${exe}.sigstore.json" "$destination/ycode-runner.sigstore.json" || die "cannot install bundle"
done
trap - EXIT HUP INT TERM
rm -rf "$stage"

echo "fetch-signed-runner: ${os}/${arch} verified"
echo "  identity: $RUNNER_IDENTITY"
echo "  executable sha256: $exe_sha"
echo "  wrote: $embed_dir/ycode-runner.gz + ycode-runner.sigstore.json"
if [ "$build_embed_dir" != "$embed_dir" ]; then
  echo "  build module: $build_embed_dir/ycode-runner.gz + ycode-runner.sigstore.json"
fi
