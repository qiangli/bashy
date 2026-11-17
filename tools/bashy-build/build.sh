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
apt_snapshot=$(get apt_snapshot)
base_image=localhost/bashy-base:$version
image=localhost/bashy-build:$version-go$go_version

ctx=$(mktemp -d)
trap 'rm -rf "$ctx"' EXIT
if [ "$mode" = seed ]; then
  # The start of the chain: a released binary, packaged as it shipped.
  cp "$binary" "$ctx/bashy"
  built_with=released-binary:sha256:$(sha256sum "$binary" | cut -d' ' -f1)
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
  # Released bashy never assumes GNU make to build itself: make is shadowed
  # by a stub that fails, so any make call in the self-build breaks loudly.
  "$here/run.sh" "$sources" -- "mkdir -p /tmp/no-make && printf '#!/bin/sh\necho bashy self-build must not call make >&2\nexit 127\n' > /tmp/no-make/make && chmod +x /tmp/no-make/make && cp /tmp/no-make/make /tmp/no-make/gmake && export PATH=/tmp/no-make:\$PATH && cd bashy && CGO_ENABLED=0 VERSION=$version BUILD_ID=$build_id bashy dag build && test ! -e bin/bashy.real"
  cp "$sources/bashy/bin/bashy" "$ctx/bashy"
  built_with=$previous_image@$previous_digest
fi
cp "$here/Containerfile.base" "$ctx/Containerfile"
$oci build -t "$base_image" \
  --build-arg VERSION="$version" \
  --build-arg APT_SNAPSHOT="$apt_snapshot" \
  --build-arg BUILT_WITH="$built_with" \
  "$ctx"

base_digest=$(digest_of "$base_image")
$oci build -t "$image" -f "$here/Containerfile" \
  --build-arg BASE_IMAGE="$base_image@$base_digest" \
  --build-arg APT_SNAPSHOT="$apt_snapshot" \
  --build-arg GO_VERSION="$go_version" \
  --build-arg GO_SHA256_AMD64="$(get go_sha256_amd64)" \
  --build-arg GO_SHA256_ARM64="$(get go_sha256_arm64)" \
  --build-arg IMAGE_ID="$image" \
  "$here"
image_digest=$(digest_of "$image")

# Smoke: the image runs bashy as its shell and has the toolchain.
$oci run --rm "$image@$image_digest" -c 'bashy --version | head -1; go version; cc --version | head -1; test -n "$BASHY_BUILD_IMAGE"'

cat <<EOF
# image.lock values for $version (commit these):
bashy_version=$version
bashy_sha256=$(sha256sum "$ctx/bashy" | cut -d' ' -f1)
previous_version=$previous
built_with_digest=$built_with
base_image=$base_image
base_digest=$base_digest
image=$image
image_digest=$image_digest
EOF
if [ "$mode" = roll ] && [ -f "$sources/commits.txt" ]; then
  sed 's/^/source_/' "$sources/commits.txt"
fi
