#!/bin/bash
# Offline policy checks for signed-asset refusal and draft cleanup scope.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/bin"
cat > "$fixture/bin/gh" <<'MOCK'
#!/bin/bash
set -euo pipefail
if [[ $1 == attestation && $2 == verify ]]; then
 printf '%s\n' "$*" >> "$LOG"
 [[ ${ATTEST_OK:-1} == 1 ]]
 exit
fi
if [[ $1 == api ]]; then
 case "$2" in
  */releases/tags/*) printf '{"draft":%s,"prerelease":true,"created_at":"%s","body":"%s"}\n' "$DRAFT" "$CREATED" "$MARKER";;
  */actions/runs/*) printf '%s\n' "$STARTED";;
  */git/ref/tags/*) printf '{"type":"commit","sha":"%s"}\n' "$REF_SHA";;
  *) exit 1;;
 esac
 exit
fi
if [[ $1 == release && $2 == delete ]]; then printf '%s\n' "$*" >> "$LOG"; exit; fi
exit 1
MOCK
chmod +x "$fixture/bin/gh"
cat > "$fixture/bin/curl" <<'MOCKCURL'
#!/bin/bash
printf '%s' "$HTTP_CODE"
MOCKCURL
chmod +x "$fixture/bin/curl"
export PATH="$fixture/bin:$PATH" LOG="$fixture/calls" GH_TOKEN=dummy REPO=qiangli/bashy TAG=v1.2.3-dev COMMIT=abc RUN_ID=42
export HTTP_CODE=404
"$root/scripts/verify-release-tag-vacant.sh"
export HTTP_CODE=200
if "$root/scripts/verify-release-tag-vacant.sh"; then echo 'accepted existing release' >&2; exit 1; fi
export HTTP_CODE=500
if "$root/scripts/verify-release-tag-vacant.sh"; then echo 'accepted API error' >&2; exit 1; fi
printf 'archive' > "$fixture/archive"
"$root/scripts/verify-release-provenance.sh" "$fixture/archive"
grep -F -- '--source-ref refs/tags/v1.2.3-dev --source-digest abc --deny-self-hosted-runners' "$LOG" >/dev/null
grep -F -- '--signer-workflow qiangli/bashy/.github/workflows/release.yml' "$LOG" >/dev/null
export ATTEST_OK=0
if "$root/scripts/verify-release-provenance.sh" "$fixture/archive"; then echo 'accepted unsigned archive' >&2; exit 1; fi
export ATTEST_OK=1 DRAFT=true CREATED=2026-10-02T21:20:00Z STARTED=2026-10-02T21:00:00Z REF_SHA=abc MARKER='<!-- bashy-release-run: 42 -->'
"$root/scripts/cleanup-incomplete-dev-release.sh"
grep -F 'release delete v1.2.3-dev' "$LOG" >/dev/null
: > "$LOG"; export DRAFT=false
"$root/scripts/cleanup-incomplete-dev-release.sh"
[[ ! -s $LOG ]] || { echo 'deleted published release' >&2; exit 1; }
export DRAFT=true CREATED=2026-10-02T20:00:00Z
"$root/scripts/cleanup-incomplete-dev-release.sh"
[[ ! -s $LOG ]] || { echo 'deleted older draft' >&2; exit 1; }
export CREATED=2026-10-02T21:20:00Z REF_SHA=other
"$root/scripts/cleanup-incomplete-dev-release.sh"
[[ ! -s $LOG ]] || { echo 'deleted wrong-commit draft' >&2; exit 1; }
export REF_SHA=abc MARKER='other run'
if "$root/scripts/publish-complete-dev-release.sh" prepare >"$fixture/publish-out" 2>&1; then echo 'accepted foreign draft' >&2; exit 1; fi
grep -F 'does not belong to this workflow run' "$fixture/publish-out" >/dev/null
"$root/scripts/cleanup-incomplete-dev-release.sh"
[[ ! -s $LOG ]] || { echo 'deleted unowned draft' >&2; exit 1; }
echo 'release provenance and cleanup policy: PASS'
