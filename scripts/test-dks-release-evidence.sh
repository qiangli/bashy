#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap '/bin/rm -rf "$tmp"' EXIT
fake="$tmp/kubectl"
source_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
candidate_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
pipeline="$tmp/pipeline.json"
bin="$tmp/bashy"

(cd "$root" && GOCACHE="${GOCACHE:-$tmp/go-cache}" go build -o "$bin" ./cmd/bashy)
DHNT="$bin dhnt"

cat >"$pipeline" <<EOF
{"schema":"dhnt.pipeline/v1","pipeline":"bashy-release","source":{"repository":"https://github.com/qiangli/bashy.git","commit":"abc123","sha256":"$source_sha"},"tasks":[{"id":"bash53","lane":"native","needs":[],"argv":["bashy","-c","test"],"workingDirectory":".","environment":[]}],"matrix":[{"task":"bash53","platform":{"backend":"vk-native","os":"darwin","arch":"arm64"},"inputs":[{"name":"candidate","sha256":"$candidate_sha"}],"outputs":[{"name":"tested-candidate","sha256":"$candidate_sha"}]}]}
EOF

# The native producer must translate host uname spellings into the canonical
# platform names consumed by the release gate. This caught a live Windows run
# that succeeded but reported windows_nt, making the three-platform gate reject
# valid evidence as "missing windows".
manifest=$(cd "$root" && \
  TARGET_OS=windows TARGET_ARCH=amd64 \
  ARTIFACT_URL=https://example.test/bashy.zip \
  ARTIFACT_SHA256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  ARTIFACT_PATH=bashy.exe \
  SOURCE_REF=abc123 SOURCE_SHA256="$source_sha" \
  PIPELINE_ID=bashy-release EVIDENCE_TASK=bash53 RUN_ID=windows-run \
  TRACE_ID=0123456789abcdef0123456789abcdef \
  EXECUTOR_NODE=puppy-vk-native \
  scripts/dks-native-job.sh)
for expected in \
  'windows_nt|mingw*|msys*) os=windows' \
  'x86_64|amd64) arch=amd64' \
  'observed OS $os does not match target windows' \
  '"$self" dhnt emit-run' \
  'DKS_RESULT:%s'
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
  *'get node dragon-vk-native'*'outpost\.dhnt\.io/backend'*)
    # An unlabelled node answers with the empty string, not an error.
    [ "${FAKE_NO_LABELS:-0}" = 1 ] || printf '%s' vk-native
    ;;
  *'get node dragon-vk-native'*'kubernetes\.io/os'*)
    printf '%s' darwin
    ;;
  *'get node dragon-vk-native'*'kubernetes\.io/arch'*)
    printf '%s' arm64
    ;;
  *'get pod successful-retry'*'.terminated.message'*)
    if [ "${FAKE_LEGACY:-0}" = 1 ]; then
      printf '%s\n' 'DKS_RESULT:{"schema":1,"classification":"pass","task":"bash53","os":"darwin","arch":"arm64","source_ref":"abc123"}'
    else
      printf '%s\n' 'DKS_RESULT:{"schema":"dhnt.run/v1","pipeline":"bashy-release","task":"bash53","run":"darwin-run","source":{"repository":"https://github.com/qiangli/bashy.git","commit":"abc123","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"inputs":[{"name":"candidate","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}],"executor":{"node":"dragon-vk-native","backend":"vk-native","os":"darwin","arch":"arm64"},"result":{"class":"pass","exitCode":0},"outputs":[{"name":"tested-candidate","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}],"startedAt":"2026-07-29T12:00:00Z","finishedAt":"2026-07-29T12:01:00Z","traceId":"0123456789abcdef0123456789abcdef"}'
    fi
    ;;
  *'get pods'*'app=conformance'*)
    printf '%s\n' conformance-pod
    ;;
  *'logs conformance-pod'*)
    printf '%s\n' 'Results: 86 passed, 0 failed, 0 skipped, 0 timed out'
    ;;
  *)
    printf 'unexpected fake kubectl call: %s\n' "$args" >&2
    exit 90
    ;;
esac
EOF
chmod +x "$fake"

result=$(cd "$root" && KUBECTL="$fake" DHNT="$DHNT" NS=test JOB=retry-job scripts/dks-native-result.sh)
case "$result" in
  *'PASS pod=successful-retry'*'"schema":"dhnt.run/v1"'*'"commit":"abc123"'*) ;;
  *) printf 'unexpected native result: %s\n' "$result" >&2; exit 1 ;;
esac

if (cd "$root" && FAKE_LEGACY=1 KUBECTL="$fake" DHNT="$DHNT" NS=test JOB=retry-job \
  scripts/dks-native-result.sh >/dev/null 2>&1); then
  echo "legacy DKS_RESULT was silently accepted" >&2
  exit 1
fi

# A node with no backend label makes every --expect-* a no-op, so the record
# would cross-check itself. That must fail closed, not pass quietly.
if (cd "$root" && FAKE_NO_LABELS=1 KUBECTL="$fake" DHNT="$DHNT" NS=test JOB=retry-job \
  scripts/dks-native-result.sh >/dev/null 2>&1); then
  echo "an unlabelled node was accepted without an executor cross-check" >&2
  exit 1
fi

gate=$(cd "$root" && KUBECTL="$fake" DHNT="$DHNT" NS=test \
  EXPECTED_SOURCE_REF=abc123 \
  EXPECTED_SOURCE_SHA256="$source_sha" \
  PIPELINE_FILE="$pipeline" \
  NATIVE_JOBS=retry-job \
  CONFORMANCE_JOBS=conformance \
  REQUIRED_PLATFORMS=darwin \
  scripts/dks-release-gate.sh)
case "$gate" in
  *'DKS_RELEASE_GATE:'*'"schema":"dhnt.aggregate/v1"'*'"result":"pass"'*) ;;
  *) printf 'unexpected release gate result: %s\n' "$gate" >&2; exit 1 ;;
esac
if (cd "$root" && KUBECTL="$fake" DHNT="$DHNT" NS=test \
  EXPECTED_SOURCE_REF=abc123 EXPECTED_SOURCE_SHA256="$source_sha" \
  PIPELINE_FILE="$pipeline" NATIVE_JOBS=retry-job CONFORMANCE_JOBS=conformance \
  REQUIRED_PLATFORMS=windows scripts/dks-release-gate.sh >/dev/null 2>&1); then
  echo "release gate accepted a pipeline missing a policy-required platform" >&2
  exit 1
fi

# A required evidence list containing only whitespace is still absent evidence.
# The release gate must not emit a passing authorization with zero conformance
# jobs merely because shell parameter expansion considers " " non-empty.
if (cd "$root" && KUBECTL="$fake" DHNT="$DHNT" NS=test \
  EXPECTED_SOURCE_REF=abc123 EXPECTED_SOURCE_SHA256="$source_sha" \
  PIPELINE_FILE="$pipeline" NATIVE_JOBS=retry-job CONFORMANCE_JOBS=" " \
  REQUIRED_PLATFORMS=darwin scripts/dks-release-gate.sh >/dev/null 2>&1); then
  echo "release gate accepted zero conformance jobs" >&2
  exit 1
fi

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
[ "$EXPECTED_SOURCE_SHA256" = aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ]
[ -s "$PIPELINE_FILE" ]
# The v0.19.3 three-platform policy still travels to the gate. Adding the
# pipeline plan strengthens the evidence; it does not replace the policy.
[ "$REQUIRED_PLATFORMS" = "linux darwin windows" ]
echo DKS_RELEASE_GATE:fake-pass
EOF
chmod +x "$fake_gate"

refs=$(cd "$root" && VERSION=v1.2.3 EXPECTED_SOURCE_REF=abc123 \
  EXPECTED_SOURCE_SHA256="$source_sha" PIPELINE_FILE="$pipeline" \
  NATIVE_JOBS=native CONFORMANCE_JOBS=conformance \
  GH="$fake_gh" GATE="$fake_gate" DRY_RUN=1 \
  scripts/dks-author-qa-refs.sh)
case "$refs" in
  *'DKS_QA_REFS_READY version=v1.2.3 source_ref=abc123 platforms=linux darwin windows'*) ;;
  *) printf 'unexpected QA ref result: %s\n' "$refs" >&2; exit 1 ;;
esac

echo "dks release evidence retry selection: PASS"
