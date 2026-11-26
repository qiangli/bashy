#!/bin/sh
# Build the official build image (tools/bashy-build/image.lock) as a rolling
# chain: the previous release builds the next one.
#
#   build.sh seed VERSION BINARY          # package a released binary as the start
#   build.sh roll NEW_VERSION SOURCES_DIR  # build NEW inside the locked image
#
# The rule: if the previous release cannot build the next, fix the bug in the
# new version's sources (never patch the old release), then roll the new
# version again inside itself to prove it self-hosts. SOURCES_DIR holds flat
# sibling trees (bashy, coreutils, sh, readline), optionally with commits.txt
# (name=commit lines) naming them. Run it on a gate runner, never on the dev
# box. It prints the lock values to commit; it never edits the lock itself.
set -eu
here=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
lock=${BASHY_BUILD_LOCK:-$here/image.lock}
fail() { echo "bashy-build build: $*" >&2; exit 1; }
get() { sed -n "s/^$1=//p" "$lock" | head -n 1; }
oci=${BASHY_OCI:-bashy oci}
digest_of() { $oci image inspect --format '{{.Digest}}' "$1"; }
sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

mode=${1:-}
case "$mode" in
  seed)
    [ $# -eq 3 ] || fail "usage: build.sh seed VERSION BINARY"
    version=$2
    binary=$3
    [ -x "$binary" ] || fail "no executable: $binary"
    ;;
  roll)
    [ $# -eq 3 ] || fail "usage: build.sh roll NEW_VERSION SOURCES_DIR"
    version=$2
    sources=$(CDPATH= cd -P "$3" && pwd) || fail "no sources: $3"
    for d in bashy coreutils sh readline; do
      [ -f "$sources/$d/go.mod" ] || fail "$sources/$d is not a source tree"
    done
    ;;
  *) fail "usage: build.sh seed VERSION BINARY | roll NEW_VERSION SOURCES_DIR" ;;
esac

go_version=$(get go_version)
go_sha256_amd64=$(get go_sha256_amd64)
go_sha256_arm64=$(get go_sha256_arm64)
apt_snapshot=$(get apt_snapshot)
base_image=localhost/bashy-base:$version
image=localhost/bashy-build:$version-go$go_version

ctx=$(mktemp -d)
trap 'rm -rf "$ctx"' EXIT
if [ "$mode" = seed ]; then
  # The start of the chain: a released binary, packaged as it shipped.
  cp "$binary" "$ctx/bashy"
  built_with=released-binary:sha256:$(sha256_file "$binary")
  previous=
else
  previous=$(get bashy_version)
  previous_image=$(get image)
  previous_digest=$(get image_digest)
  [ -n "$previous_digest" ] || fail "the lock has no image_digest for $previous; seed or roll it first"
  # Strictly upon the previous version: the previous bashy (the image's shell)
  # builds the new one with bashy's own dag build target (Bash#, no make), in
  # the release shape: one static Go file, stamped with version and commit.
  build_id=
  [ -f "$sources/commits.txt" ] && build_id=$(sed -n 's/^bashy_commit=//p' "$sources/commits.txt" | cut -c1-7)
  if [ -n "${BASHY_GO_TARBALL:-}" ]; then
    [ -f "$BASHY_GO_TARBALL" ] || fail "missing BASHY_GO_TARBALL: $BASHY_GO_TARBALL"
    case "${BASHY_GO_ARCH:-$(uname -m)}" in
      x86_64|amd64) want_go_sum=$go_sha256_amd64 ;;
      aarch64|arm64) want_go_sum=$go_sha256_arm64 ;;
      *) fail "unsupported arch ${BASHY_GO_ARCH:-$(uname -m)}" ;;
    esac
    got_go_sum=$(sha256_file "$BASHY_GO_TARBALL")
    [ "$got_go_sum" = "$want_go_sum" ] || fail "BASHY_GO_TARBALL sha256 $got_go_sum != locked $want_go_sum"
  fi
  # Released bashy never assumes GNU make to build itself: make is shadowed
  # by a stub that fails, so any make call in the self-build breaks loudly.
  # When rolling across a go_version bump, the previous release's image carries
  # the older Go SDK; stage the checksum-locked go_version SDK into binmgr's
  # tree cache so `bashy go` inside the builder uses the locked target SDK.
  go_dl_base=${BASHY_GO_DL_BASE:-https://go.dev/dl}
  if [ "${BASHY_OFFLINE:-0}" = "1" ]; then
    export BASHY_BUILD_OFFLINE=1
  fi
  "$here/run.sh" "$sources" -- "set -eu
mkdir -p /tmp/no-make
printf '#!/bin/sh\necho bashy self-build must not call make >&2\nexit 127\n' > /tmp/no-make/make
chmod +x /tmp/no-make/make
cp /tmp/no-make/make /tmp/no-make/gmake
go_cache=\${BASHY_BIN_CACHE:-/var/cache/bashy/bin}/go/$go_version
if [ ! -x \"\$go_cache/go/bin/go\" ] || [ ! -f \"\$go_cache/.binmgr-tree-complete\" ]; then
  case \"\$(uname -m)\" in
    x86_64|amd64) arch=amd64; sum='$go_sha256_amd64' ;;
    aarch64|arm64) arch=arm64; sum='$go_sha256_arm64' ;;
    *) echo \"unsupported arch \$(uname -m)\" >&2; exit 1 ;;
  esac
  if [ -n \"\${BASHY_GO_TARBALL:-}\" ]; then
    [ -f \"\$BASHY_GO_TARBALL\" ] || { echo \"missing BASHY_GO_TARBALL: \$BASHY_GO_TARBALL\" >&2; exit 1; }
    echo \"\$sum  \$BASHY_GO_TARBALL\" | sha256sum -c - >/dev/null
    rm -rf \"\$go_cache\"
    mkdir -p \"\$go_cache\"
    tar -C \"\$go_cache\" -xzf \"\$BASHY_GO_TARBALL\"
    printf 'ok\n' > \"\$go_cache/.binmgr-tree-complete\"
  elif [ \"\${BASHY_OFFLINE:-0}\" = \"1\" ]; then
    echo \"bashy-build build: BASHY_OFFLINE=1 refuses network download from '$go_dl_base'; pre-seed \$go_cache or set BASHY_GO_TARBALL\" >&2
    exit 1
  else
    curl -fsSL -o /tmp/go-roll.tgz '$go_dl_base/go$go_version.linux-'\"\$arch\"'.tar.gz'
    echo \"\$sum  /tmp/go-roll.tgz\" | sha256sum -c - >/dev/null
    rm -rf \"\$go_cache\"
    mkdir -p \"\$go_cache\"
    tar -C \"\$go_cache\" -xzf /tmp/go-roll.tgz
    rm -f /tmp/go-roll.tgz
    printf 'ok\n' > \"\$go_cache/.binmgr-tree-complete\"
  fi
fi
export BASHY_GO_VERSION='$go_version'
export PATH=\"\$go_cache/go/bin:/tmp/no-make:\$PATH\"
cd bashy
GOWORK=off CGO_ENABLED=0 VERSION='$version' BUILD_ID='$build_id' bashy dag build
test ! -e bin/bashy.real"
  cp "$sources/bashy/bin/bashy" "$ctx/bashy"
  built_with=$previous_image@$previous_digest
fi
cp "$here/Containerfile.base" "$ctx/Containerfile"
oci_build_flags=
oci_run_flags=
if [ "${BASHY_OFFLINE:-0}" = "1" ]; then
  oci_build_flags="--pull=never --network=none"
  oci_run_flags="--network=none"
fi
# shellcheck disable=SC2086
$oci build $oci_build_flags -t "$base_image" \
  --build-arg VERSION="$version" \
  --build-arg APT_SNAPSHOT="$apt_snapshot" \
  --build-arg BUILT_WITH="$built_with" \
  "$ctx"

base_digest=$(digest_of "$base_image")
# shellcheck disable=SC2086
$oci build $oci_build_flags -t "$image" -f "$here/Containerfile" \
  --build-arg BASE_IMAGE="$base_image@$base_digest" \
  --build-arg APT_SNAPSHOT="$apt_snapshot" \
  --build-arg GO_VERSION="$go_version" \
  --build-arg GO_SHA256_AMD64="$(get go_sha256_amd64)" \
  --build-arg GO_SHA256_ARM64="$(get go_sha256_arm64)" \
  --build-arg IMAGE_ID="$image" \
  "$here"
image_digest=$(digest_of "$image")
# A unique tag per build: a later build reuses $image, and an untagged image
# cannot be found locally by name@digest, so the lock names this tag.
unique=$image-$(echo "$image_digest" | sed 's/^sha256://' | cut -c1-12)
$oci tag "$image" "$unique"

# Smoke: the image runs bashy as its shell and has the toolchain.
# shellcheck disable=SC2086
$oci run --rm $oci_run_flags "$image@$image_digest" -c 'bashy --version | head -1; go version; cc --version | head -1; test -n "$BASHY_BUILD_IMAGE"'

cat <<EOF
# image.lock values for $version (commit these):
bashy_version=$version
bashy_sha256=$(sha256_file "$ctx/bashy")
previous_version=$previous
built_with_digest=$built_with
base_image=$base_image
base_digest=$base_digest
image=$unique
image_digest=$image_digest
EOF
if [ "$mode" = roll ] && [ -f "$sources/commits.txt" ]; then
  sed 's/^/source_/' "$sources/commits.txt"
fi
