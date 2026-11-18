#!/bin/bash
# Run once on the frozen source in the locked local build image, not on Actions.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ ${BASHY_BUILD_RUN:-} == 1 && -n ${BASHY_BUILD_IMAGE:-} ]] || { echo 'local release gate requires the official build-image runner' >&2; exit 1; }
[[ -z $(git status --porcelain --untracked-files=all) ]] || { echo 'freeze and commit the source before collecting release evidence' >&2; exit 1; }
export GOWORK=off
out=${LOCAL_RELEASE_GATE_OUT:-bin/local-release-gate}
mkdir -p "$out"
./scripts/resolve-pinned-siblings.sh
LICENSE_GATE_REPORT_ONLY=0 LICENSE_GATE_OUT="$out/license" ./scripts/verify-license-gate.sh
GOVULNCHECK_OUT="$out/govulncheck" ./scripts/govulncheck-release.sh . ../outpost ../yoke
dist_args=()
[[ ! -f ${LOCAL_DIST_MANIFEST:-dist/release-assets.json} ]] || dist_args=(--dist-manifest "${LOCAL_DIST_MANIFEST:-dist/release-assets.json}")
python3 scripts/local-release-evidence.py collect "$out/evidence.json" --reports "$out" --scope . --scope ../outpost --scope ../yoke "${dist_args[@]}"
# No notes/branch/tag mutation here. The manager reviews and commits this receipt
# to refs/notes/bashy-local-gates, then approves that notes commit in repository
# dispatch input evidence_commit before publication.
