#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap '/bin/rm -rf "$tmp"' EXIT
adversarial_fail=0
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
    if [ "${FAKE_DUPLICATE_COMPLETION_INDEX:-0}" = 1 ]; then
      printf '%s\n' conformance-index-0-a conformance-index-0-b
    elif [ "${FAKE_FOREIGN_CONFORMANCE_POD:-0}" = 1 ]; then
      printf '%s\n' foreign-conformance-pod
    else
      printf '%s\n' conformance-pod
    fi
    ;;
  *'get pod conformance-pod'*'.metadata.ownerReferences'*'.uid'*)
    printf '%s' conformance-job-uid
    ;;
  *'get pod conformance-pod'*'batch\.kubernetes\.io/job-completion-index'*)
    printf '%s' 0
    ;;
  *'get pod conformance-index-0-'*'.metadata.ownerReferences'*'.uid'*)
    printf '%s' conformance-job-uid
    ;;
  *'get pod conformance-index-0-'*'batch\.kubernetes\.io/job-completion-index'*)
    printf '%s' 0
    ;;
  *'get pod foreign-conformance-pod'*'.metadata.ownerReferences'*'.uid'*)
    printf '%s' foreign-job-uid
    ;;
  *'get pod foreign-conformance-pod'*'batch\.kubernetes\.io/job-completion-index'*)
    printf '%s' 0
    ;;
  *'get job conformance'*'.metadata.uid'*)
    printf '%s' conformance-job-uid
    ;;
  *'get job conformance'*)
    if [ "${FAKE_JOB_QUERY_FAIL:-0}" = 1 ]; then
      echo "error: kubectl: connection refused" >&2
      exit 1
    fi
    if [ "${FAKE_JOB_EMPTY:-0}" = 1 ]; then
      printf '\n'
    elif [ "${FAKE_JOB_MALFORMED:-0}" = 1 ]; then
      printf '%s\n' '{"metadata":{"uid":"conformance-job-uid"},"spec":{"completions":1},"status":{"succeeded":1}}'
    elif [ "${FAKE_JOB_NON_INDEXED:-0}" = 1 ]; then
      printf '%s\n' '{"metadata":{"uid":"conformance-job-uid"},"spec":{"completions":1,"completionMode":"NonIndexed"},"status":{"succeeded":1}}'
    elif [ "${FAKE_JOB_MISSING_COMPLETIONS:-0}" = 1 ]; then
      printf '%s\n' '{"metadata":{"uid":"conformance-job-uid"},"spec":{"completionMode":"Indexed"},"status":{"succeeded":1}}'
    elif [ "${FAKE_JOB_MISSING_SUCCEEDED:-0}" = 1 ]; then
      printf '%s\n' '{"metadata":{"uid":"conformance-job-uid"},"spec":{"completions":2,"completionMode":"Indexed"},"status":{}}'
    elif [ "${FAKE_JOB_ZERO_SUCCEEDED:-0}" = 1 ]; then
      printf '%s\n' '{"metadata":{"uid":"conformance-job-uid"},"spec":{"completions":2,"completionMode":"Indexed"},"status":{"succeeded":0}}'
    elif [ "${FAKE_INCOMPLETE_JOB:-0}" = 1 ]; then
      printf '%s\n' '{"metadata":{"uid":"conformance-job-uid"},"spec":{"completions":2,"completionMode":"Indexed"},"status":{"succeeded":1}}'
    elif [ "${FAKE_UNOBSERVED_COMPLETION:-0}" = 1 ]; then
      printf '%s\n' '{"metadata":{"uid":"conformance-job-uid"},"spec":{"completions":2,"completionMode":"Indexed"},"status":{"succeeded":2}}'
    elif [ "${FAKE_DUPLICATE_COMPLETION_INDEX:-0}" = 1 ]; then
      printf '%s\n' '{"metadata":{"uid":"conformance-job-uid"},"spec":{"completions":2,"completionMode":"Indexed"},"status":{"succeeded":2,"completedIndexes":"0,1"}}'
    else
      printf '%s\n' '{"metadata":{"uid":"conformance-job-uid"},"spec":{"completions":1,"completionMode":"Indexed"},"status":{"succeeded":1}}'
    fi
    ;;
  *'logs conformance-pod'*|*'logs conformance-index-0-'*|*'logs foreign-conformance-pod'*)
    if [ "${FAKE_MALFORMED_RESULTS:-0}" = 1 ]; then
      printf '%s\n' 'Results: 86 passed'
    elif [ "${FAKE_AMBIGUOUS_RESULTS:-0}" = 1 ]; then
      printf '%s\n' 'Results: 0 passed, 1 failed, 0 skipped, 0 timed out; 86 passed, 0 failed, 0 skipped, 0 timed out'
    elif [ "${FAKE_MULTIPLE_RESULTS:-0}" = 1 ]; then
      printf '%s\n' \
        'Results: 0 passed, 1 failed, 0 skipped, 0 timed out' \
        'Results: 86 passed, 0 failed, 0 skipped, 0 timed out'
    elif [ "${FAKE_ZERO_TESTS:-0}" = 1 ]; then
      printf '%s\n' 'Results: 0 passed, 0 failed, 0 skipped, 0 timed out'
    elif [ "${FAKE_ALL_SKIPPED:-0}" = 1 ]; then
      printf '%s\n' 'Results: 0 passed, 0 failed, 86 skipped, 0 timed out'
    elif [ "${FAKE_PARTIAL_SKIPS:-0}" = 1 ]; then
      printf '%s\n' 'Results: 1 passed, 0 failed, 85 skipped, 0 timed out'
    else
      printf '%s\n' 'Results: 86 passed, 0 failed, 0 skipped, 0 timed out'
    fi
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

# An empty conformance run is absence of evidence, even when its syntactically
# valid summary contains no failures.
if (cd "$root" && FAKE_ZERO_TESTS=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted a run that executed zero tests" >&2
  exit 1
fi

# Skipped cases were discovered but not executed, so an all-skipped summary is
# still absence of evidence rather than a passing conformance run.
if (cd "$root" && FAKE_ALL_SKIPPED=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted a run in which every test was skipped" >&2
  exit 1
fi

# Executing one case does not turn the other 85 skipped cases into evidence.
# The authoritative Bash 5.3 release contract measures the whole suite and
# requires zero skips.
if (cd "$root" && FAKE_PARTIAL_SKIPS=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted a partial run with skipped tests" >&2
  adversarial_fail=1
fi

# One successful chunk is not complete evidence for a two-completion Indexed
# Job. The other chunk may still be running or may have exhausted its retries.
if (cd "$root" && FAKE_INCOMPLETE_JOB=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted an incomplete Indexed Job" >&2
  adversarial_fail=1
fi

# Job status is not a substitute for collected evidence. If two completions are
# required but only one successful pod contributes a Results record, the
# aggregate is incomplete even when status.succeeded claims two.
if (cd "$root" && FAKE_UNOBSERVED_COMPLETION=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted one observed result for two required completions" >&2
  adversarial_fail=1
fi

# Indexed Job completeness is per completion index, not per successful Pod.
# Kubernetes can briefly retain two successful retries for the same index.
# Two index-0 summaries cannot substitute for the absent index-1 evidence,
# even when Job status says both indexes eventually completed.
if (cd "$root" && FAKE_DUPLICATE_COMPLETION_INDEX=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted duplicate evidence for completion index 0 while index 1 was absent" >&2
  adversarial_fail=1
fi

# The app label is settable by any Pod in the namespace and does not prove Job
# ownership. A passing summary from a foreign Pod must not authorize this Job
# when the Job's own evidence is absent.
if (cd "$root" && FAKE_FOREIGN_CONFORMANCE_POD=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted evidence from a Pod not owned by the Job" >&2
  adversarial_fail=1
fi

# kubectl get job query failure must fail closed — an unreachable API is
# not a signal that the Job is complete.
if (cd "$root" && FAKE_JOB_QUERY_FAIL=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted an unreachable kubectl API" >&2
  exit 1
fi

# An empty kubectl response is not valid Job JSON. The aggregator must not
# treat missing data as an implicit successful completion.
if (cd "$root" && FAKE_JOB_EMPTY=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted an empty kubectl response" >&2
  exit 1
fi

# Missing completionMode in the Job object means we cannot verify that this
# is an Indexed Job. Completeness is defined only for Indexed Jobs.
if (cd "$root" && FAKE_JOB_MALFORMED=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted a Job with missing completionMode" >&2
  exit 1
fi

# A non-Indexed completionMode means the Job did not use the completionMode
# contract that the aggregator expects. Its pod count cannot be verified.
if (cd "$root" && FAKE_JOB_NON_INDEXED=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted a non-Indexed Job" >&2
  exit 1
fi

# Missing spec.completions means we do not know how many pods should have
# run. Without a required-completions count, completeness is unknowable.
if (cd "$root" && FAKE_JOB_MISSING_COMPLETIONS=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted a Job with missing spec.completions" >&2
  exit 1
fi

# Missing status.succeeded means we do not know how many pods completed.
# Without that count, completeness is unknowable.
if (cd "$root" && FAKE_JOB_MISSING_SUCCEEDED=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted a Job with missing status.succeeded" >&2
  exit 1
fi

# status.succeeded must be positive when completions > 0 — zero succeeded
# with nonzero required completions means no pods produced evidence.
if (cd "$root" && FAKE_JOB_ZERO_SUCCEEDED=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted a Job with zero succeeded pods" >&2
  exit 1
fi

# Missing counters are malformed evidence, not implicit zeroes. In particular,
# a truncated line that retains only a positive pass count must not authorize a
# passing conformance verdict.
if (cd "$root" && FAKE_MALFORMED_RESULTS=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted a truncated Results record" >&2
  exit 1
fi

# Parse exactly one complete summary. Greedy per-counter extraction must not
# let a later passing suffix erase an earlier failure on the same malformed
# evidence line.
if (cd "$root" && FAKE_AMBIGUOUS_RESULTS=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted an ambiguous Results record containing a failure" >&2
  exit 1
fi

# A pod must contribute exactly one summary. Selecting the final line lets a
# later passing record erase an earlier failure from the same pod log.
if (cd "$root" && FAKE_MULTIPLE_RESULTS=1 KUBECTL="$fake" NS=test JOB=conformance \
  scripts/k8s-job-aggregate.sh >/dev/null 2>&1); then
  echo "conformance aggregation accepted multiple Results records from one pod" >&2
  adversarial_fail=1
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

# Whitespace is not a platform policy. Without at least one parsed required OS,
# the independent release policy has disappeared even though the variable is
# technically non-empty.
if (cd "$root" && KUBECTL="$fake" DHNT="$DHNT" NS=test \
  EXPECTED_SOURCE_REF=abc123 EXPECTED_SOURCE_SHA256="$source_sha" \
  PIPELINE_FILE="$pipeline" NATIVE_JOBS=retry-job CONFORMANCE_JOBS=conformance \
  REQUIRED_PLATFORMS=" " scripts/dks-release-gate.sh >/dev/null 2>&1); then
  echo "release gate accepted an empty required-platform policy" >&2
  adversarial_fail=1
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

[ "$adversarial_fail" -eq 0 ] || exit 1
echo "dks release evidence retry selection: PASS"
