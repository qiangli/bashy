#!/usr/bin/env bash
# verify-pin.sh SRC_DIR [BASHY_TAG]
#
# Fail-closed provenance gate. Refuses to build unless:
#   - the pinned upstream tag is a plain release in the form the declaration
#     chose (UPSTREAM_TAG_FORM: vMAJOR.MINOR.PATCH by default, bare
#     MAJOR.MINOR for an upstream that tags that way) — no
#     -rc/-beta/-dev/pre-release suffix either way;
#   - the bashy tag (when given) is bashy-${UPSTREAM_TAG}-N;
#   - SRC_DIR's HEAD is exactly UPSTREAM_COMMIT and the tag resolves to it;
#   - when PIN declares RUST_TOOLCHAIN, the source's rust-toolchain.toml
#     channel equals it.
#
# The gate does not care which forge SRC_DIR came from: an actions/checkout of
# a GitHub upstream and a `git clone` of a non-GitHub one are held to the same
# commit, tag and pristine-tree requirements.
#
# Shared by every builds/<tool> (builds/_shared; rendered by builds/render.sh,
# which substitutes the release-tag regex for the declared form).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$here/PIN"

src=${1:?usage: verify-pin.sh SRC_DIR [BASHY_TAG]}
bashy_tag=${2:-}

die() { echo "verify-pin: $*" >&2; exit 1; }

# The shape of the declared UPSTREAM_TAG_FORM, rendered from builds/render.sh.
upstream_tag_re='^v[0-9]+\.[0-9]+\.[0-9]+$'
[[ $UPSTREAM_TAG =~ $upstream_tag_re ]] ||
	die "upstream tag $UPSTREAM_TAG is not a plain vX.Y.Z release tag (rc/pre-release/dev refused)"
[[ $UPSTREAM_COMMIT =~ ^[0-9a-f]{40}$ ]] || die "UPSTREAM_COMMIT must be a full 40-hex sha"

if [[ -n $bashy_tag ]]; then
	# bashy-<the pinned upstream tag>-N, whatever shape that tag has: the
	# middle is matched against UPSTREAM_TAG, which was just gated above, so
	# this needs no second copy of the form.
	[[ $bashy_tag =~ ^bashy-(.+)-([1-9][0-9]*)$ ]] ||
		die "bashy tag $bashy_tag must be bashy-$UPSTREAM_TAG-N"
	[[ ${BASH_REMATCH[1]} == "$UPSTREAM_TAG" ]] ||
		die "bashy tag $bashy_tag does not build the pinned upstream $UPSTREAM_TAG"
fi

head=$(git -C "$src" rev-parse HEAD)
[[ $head == "$UPSTREAM_COMMIT" ]] || die "HEAD $head != pinned $UPSTREAM_COMMIT"
tagged=$(git -C "$src" rev-parse "refs/tags/$UPSTREAM_TAG^{commit}") ||
	die "tag $UPSTREAM_TAG missing from checkout"
[[ $tagged == "$UPSTREAM_COMMIT" ]] || die "tag $UPSTREAM_TAG -> $tagged != pinned $UPSTREAM_COMMIT"
[[ -z $(git -C "$src" status --porcelain) ]] || die "upstream checkout is not pristine"

note=""
if [[ -n ${RUST_TOOLCHAIN:-} ]]; then
	channel=$(sed -n 's/^channel *= *"\(.*\)"/\1/p' "$src/rust-toolchain.toml")
	[[ $channel == "$RUST_TOOLCHAIN" ]] || die "rust-toolchain.toml channel $channel != pinned $RUST_TOOLCHAIN"
	note=" (toolchain $RUST_TOOLCHAIN)"
fi

echo "verify-pin: $UPSTREAM_SLUG $UPSTREAM_TAG = $UPSTREAM_COMMIT$note OK"
