#!/bin/bash
# Read-only preflight. A floating notes ref alone is not an approval authority.
set -euo pipefail
cd "$(dirname "$0")/.."
: "${COMMIT:?exact candidate commit required}" "${LOCAL_GATE_EVIDENCE_COMMIT:?approved notes commit required}"
[[ $COMMIT =~ ^[0-9a-f]{40}$ && $LOCAL_GATE_EVIDENCE_COMMIT =~ ^[0-9a-f]{40}$ ]] || { echo 'expected full commit identifiers' >&2; exit 1; }
[[ $(git rev-parse HEAD) == "$COMMIT" ]] || { echo 'checkout is not the candidate commit' >&2; exit 1; }
[[ -z $(git status --porcelain --untracked-files=all) ]] || { echo 'candidate checkout is dirty' >&2; exit 1; }
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
# Only origin (the checkout repository), and only an explicitly approved immutable
# notes commit. Later/untrusted updates to the notes ref fail closed.
git fetch --no-tags origin refs/notes/bashy-local-gates:refs/notes/bashy-local-gates
[[ $(git rev-parse FETCH_HEAD) == "$LOCAL_GATE_EVIDENCE_COMMIT" ]] || { echo 'local gate notes differ from manager-approved commit' >&2; exit 1; }
git notes --ref=refs/notes/bashy-local-gates show "$COMMIT" > "$work/evidence.json"
python3 scripts/local-release-evidence.py verify "$work/evidence.json" --commit "$COMMIT"

[[ -z ${LOCAL_GATE_EVIDENCE_OUT:-} ]] || cp "$work/evidence.json" "$LOCAL_GATE_EVIDENCE_OUT"
