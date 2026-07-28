#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap '/bin/rm -rf "$tmp"' EXIT
fake="$tmp/kubectl"

cat >"$fake" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
args="$*"
case "$args" in
  *'get pods'*'status.phase=="Succeeded"'*)
    printf '%s\n' successful-retry
    ;;
  *'get pods'*'.items[0].metadata.name'*)
    # The unordered first item is deliberately the failed attempt.
    printf '%s\n' failed-first
    ;;
  *'get pod successful-retry'*'.status.phase'*)
    printf '%s' Succeeded
    ;;
  *'get pod successful-retry'*'.spec.nodeName'*)
    printf '%s' dragon-vk-native
    ;;
  *'get pod successful-retry'*'.terminated.message'*)
    printf '%s\n' 'DKS_RESULT:{"schema":1,"classification":"pass","lane":"native-platform","task":"bash53","os":"darwin","arch":"arm64","version":"test","source_ref":"abc123"}'
    ;;
  *)
    printf 'unexpected fake kubectl call: %s\n' "$args" >&2
    exit 90
    ;;
esac
EOF
chmod +x "$fake"

result=$(cd "$root" && KUBECTL="$fake" NS=test JOB=retry-job scripts/dks-native-result.sh)
case "$result" in
  *'PASS pod=successful-retry'*'"source_ref":"abc123"'*) ;;
  *) printf 'unexpected native result: %s\n' "$result" >&2; exit 1 ;;
esac

gate=$(cd "$root" && KUBECTL="$fake" NS=test \
  EXPECTED_SOURCE_REF=abc123 \
  NATIVE_JOBS=retry-job \
  CONFORMANCE_JOBS=" " \
  REQUIRED_PLATFORMS=darwin \
  scripts/dks-release-gate.sh)
case "$gate" in
  *'DKS_RELEASE_GATE:'*'"source_ref":"abc123"'*) ;;
  *) printf 'unexpected release gate result: %s\n' "$gate" >&2; exit 1 ;;
esac

echo "dks release evidence retry selection: PASS"
