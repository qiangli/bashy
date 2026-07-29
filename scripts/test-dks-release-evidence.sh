#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap '/bin/rm -rf "$tmp"' EXIT
fake="$tmp/kubectl"

# The native producer must translate host uname spellings into the canonical
# platform names consumed by the release gate. This caught a live Windows run
# that succeeded but reported windows_nt, making the three-platform gate reject
# valid evidence as "missing windows".
manifest=$(cd "$root" && \
  TARGET_OS=windows TARGET_ARCH=amd64 \
  ARTIFACT_URL=https://example.test/bashy.zip \
  ARTIFACT_SHA256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  ARTIFACT_PATH=bashy.exe \
  scripts/dks-native-job.sh)
for expected in \
  'windows_nt|mingw*|msys*) os=windows' \
  'x86_64|amd64) arch=amd64' \
  'observed OS $os does not match target windows'
do
  case "$manifest" in
    *"$expected"*) ;;
    *) printf 'native job omits canonical platform check: %s\n' "$expected" >&2; exit 1 ;;
  esac
done

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

fake_gh="$tmp/gh"
cat >"$fake_gh" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *'/releases/tags/v1.2.3-dev'*) printf '%s\n' true ;;
  *'/git/ref/tags/v1.2.3-dev'*) printf '%s\n' 'commit abc123' ;;
  *) printf 'unexpected fake gh call: %s\n' "$*" >&2; exit 91 ;;
esac
EOF
chmod +x "$fake_gh"
fake_gate="$tmp/gate"
cat >"$fake_gate" <<'EOF'
#!/usr/bin/env bash
set -e
[ "$EXPECTED_SOURCE_REF" = abc123 ]
[ "$REQUIRED_PLATFORMS" = "linux darwin windows" ]
echo DKS_RELEASE_GATE:fake-pass
EOF
chmod +x "$fake_gate"

refs=$(cd "$root" && VERSION=v1.2.3 EXPECTED_SOURCE_REF=abc123 \
  NATIVE_JOBS=native CONFORMANCE_JOBS=conformance \
  GH="$fake_gh" GATE="$fake_gate" DRY_RUN=1 \
  scripts/dks-author-qa-refs.sh)
case "$refs" in
  *'DKS_QA_REFS_READY version=v1.2.3 source_ref=abc123 platforms=linux darwin windows'*) ;;
  *) printf 'unexpected QA ref result: %s\n' "$refs" >&2; exit 1 ;;
esac

echo "dks release evidence retry selection: PASS"
