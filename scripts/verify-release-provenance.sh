#!/bin/bash
# Require a GitHub-signed SLSA provenance claim for the exact artifact bytes,
# source tag/commit and this repository's release workflow.
set -euo pipefail
[[ $# == 1 && -f $1 ]] || { echo "usage: $0 ARTIFACT" >&2; exit 2; }
: "${REPO:?}" "${TAG:?}" "${COMMIT:?}"
gh attestation verify "$1" -R "$REPO" \
  --signer-workflow "$REPO/.github/workflows/release.yml" \
  --source-ref "refs/tags/$TAG" --source-digest "$COMMIT" \
  --deny-self-hosted-runners >/dev/null
echo "release provenance: PASS $(basename "$1")"
