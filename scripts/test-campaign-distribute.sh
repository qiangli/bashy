#!/usr/bin/env bash
# Deterministic tests for scripts/campaign-distribute.sh and its real peer-DKS
# transport scripts/campaign-distribute-k8s.sh. No cluster, no network:
#
#  - the reduction/equivalence logic is exercised through the explicit
#    CAMPAIGN_TEST_FAKE_TRANSPORT unit-injection seam (a local fake executor);
#  - the k8s transport is exercised end-to-end (preflight, Job manifest,
#    placement/identity evidence, collection, cleanup) through a FAKE kubectl
#    injected via CAMPAIGN_K8S_FAKE_KUBECTL that simulates a small peer
#    cluster on disk.
#
# Neither seam is a real execution mode: both force the non-promotable
# CAMPAIGN_FAKE_TRANSPORT_VERDICT marker, and this file asserts that. Real
# cross-worker execution remains UNPROVEN on hardware — see
# docs/distributed-campaign-verdict-equivalence.md.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
DISTRIBUTE="$root/scripts/campaign-distribute.sh"
K8S="$root/scripts/campaign-distribute-k8s.sh"
tmp=$(mktemp -d)
trap '/bin/rm -rf "$tmp"' EXIT
adversarial_fail=0

fail() { printf '%s\n' "$1" >&2; exit 1; }
adversarial() { printf '%s\n' "$1" >&2; adversarial_fail=1; }

# --- corpus + canonical outcome map -----------------------------------------
cases="$tmp/cases.txt"
printf '%s\n' case-a case-b case-c case-d case-e case-f >"$cases"

outcomes="$tmp/outcomes.tsv"
cat >"$outcomes" <<'EOF'
case-a pass
case-b fail
case-c pass
case-d pass
case-e skip
case-f pass
EOF

# --- fake per-chunk executor: $1=chunk_cases_file $2=worker -----------------
# Reads the canonical outcome for each case in the chunk from $OUTCOME_MAP,
# with fault-injection knobs (all off by default) used by the adversarial
# tests below. Never touches the network or a real cluster.
fake_run_chunk="$tmp/fake-run-chunk.sh"
cat >"$fake_run_chunk" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
chunk_file="$1"
worker="$2"

if [ -n "${FAKE_UNREACHABLE_WORKER:-}" ] && [ "$worker" = "$FAKE_UNREACHABLE_WORKER" ]; then
  echo "fake-run-chunk: simulated unreachable worker $worker" >&2
  exit 9
fi

if [ -n "${FAKE_EMPTY_WORKER:-}" ] && [ "$worker" = "$FAKE_EMPTY_WORKER" ]; then
  exit 0
fi

while IFS= read -r id; do
  [ -n "$id" ] || continue
  if [ "$worker" != "serial-exclusive" ] && [ "$id" = "${FAKE_DROP_CASE:-}" ]; then
    continue
  fi
  outcome=$(awk -v i="$id" '$1==i{print $2}' "$OUTCOME_MAP")
  if [ "$worker" != "serial-exclusive" ] && [ -n "${FAKE_SWAP_A:-}" ] && [ -n "${FAKE_SWAP_B:-}" ]; then
    if [ "$id" = "$FAKE_SWAP_A" ]; then
      outcome=$(awk -v i="$FAKE_SWAP_B" '$1==i{print $2}' "$OUTCOME_MAP")
    elif [ "$id" = "$FAKE_SWAP_B" ]; then
      outcome=$(awk -v i="$FAKE_SWAP_A" '$1==i{print $2}' "$OUTCOME_MAP")
    fi
  fi
  printf '%s %s\n' "$id" "$outcome"
done <"$chunk_file"

if [ "$worker" != "serial-exclusive" ] && [ -n "${FAKE_INJECT_FOREIGN_CASE:-}" ]; then
  printf '%s %s\n' "$FAKE_INJECT_FOREIGN_CASE" pass
fi
FAKE
chmod +x "$fake_run_chunk"

export OUTCOME_MAP="$outcomes"
export SUITE=free-utils-sweep
export CASES_FILE="$cases"
export RUN_CHUNK_CMD="$fake_run_chunk"

# The unit-injection seam is opted into per-invocation, never exported
# globally — a test that forgets it must hit the no-transport refusal.
FAKESEAM=CAMPAIGN_TEST_FAKE_TRANSPORT=1

# =============================================================================
# 1. Certification line: MODE=cert refuses to distribute, unconditionally,
#    before anything else runs — with a valid config, with the fake seam,
#    with the k8s transport, and in the k8s transport script itself.
# =============================================================================
set +e
out=$(cd "$root" && MODE=cert CHUNKS=3 WORKERS="worker-a worker-b" \
  "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -eq 77 ] || fail "cert mode did not refuse with exit 77 (got $rc): $out"
case "$out" in *REFUSED*) ;; *) fail "cert mode refusal did not say REFUSED: $out" ;; esac

set +e
out=$(cd "$root" && MODE=cert "$DISTRIBUTE" serial 2>&1)
rc=$?
set -e
[ "$rc" -eq 77 ] || fail "cert mode did not refuse 'serial' subcommand with exit 77 (got $rc)"

set +e
out=$(cd "$root" && MODE=cert env "$FAKESEAM" CHUNKS=3 WORKERS="worker-a worker-b" \
  "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -eq 77 ] || fail "cert mode was bypassed by the fake-transport seam (got $rc)"

set +e
out=$(cd "$root" && MODE=cert CAMPAIGN_TRANSPORT=k8s CHUNKS=3 WORKERS="worker-a worker-b" \
  "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -eq 77 ] || fail "cert mode was bypassed by CAMPAIGN_TRANSPORT=k8s (got $rc)"

set +e
out=$(cd "$root" && MODE=cert "$K8S" preflight 2>&1)
rc=$?
set -e
[ "$rc" -eq 77 ] || fail "campaign-distribute-k8s.sh did not refuse MODE=cert with exit 77 (got $rc): $out"

# =============================================================================
# 2. No transport is not a transport: distribute with a fully valid config but
#    neither CAMPAIGN_TRANSPORT=k8s nor the test seam must refuse — the local
#    fake is NOT reachable as a default or named execution mode.
# =============================================================================
set +e
out=$(cd "$root" && CHUNKS=3 SEED=1 WORKERS="worker-a worker-b worker-c" \
  OUT_DIR="$tmp/notransport" "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -eq 8 ] || fail "distribute without a transport did not refuse with exit 8 (got $rc): $out"
case "$out" in *'no transport selected'*) ;; *) fail "no-transport refusal did not name the cause: $out" ;; esac

set +e
out=$(cd "$root" && CAMPAIGN_TRANSPORT=local CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/badtransport" "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -eq 8 ] || fail "CAMPAIGN_TRANSPORT=local (not a real transport) was accepted (got $rc): $out"

# =============================================================================
# 3. Manifest determinism: same (seed, corpus, chunks) -> byte-identical
#    manifest, on repeated generation.
# =============================================================================
m1="$tmp/m1.tsv"; m2="$tmp/m2.tsv"
(cd "$root" && CHUNKS=3 SEED=7 MANIFEST="$m1" "$DISTRIBUTE" manifest >/dev/null)
(cd "$root" && CHUNKS=3 SEED=7 MANIFEST="$m2" "$DISTRIBUTE" manifest >/dev/null)
diff "$m1" "$m2" >/dev/null || fail "manifest generation is not deterministic for the same seed"
[ "$(sort -u "$m1" | wc -l | tr -d ' ')" -eq 6 ] || fail "manifest does not cover all 6 cases exactly once"

# =============================================================================
# 4. Happy path distribute through the injection seam: verdict matches the
#    canonical outcome map exactly, and is marked as a FAKE-transport logic
#    check — it must NOT carry the promotable CAMPAIGN_VERDICT marker.
# =============================================================================
out_dir="$tmp/happy"
result=$(cd "$root" && env "$FAKESEAM" CHUNKS=3 SEED=1 WORKERS="worker-a worker-b worker-c" \
  OUT_DIR="$out_dir" "$DISTRIBUTE" distribute)
case "$result" in
  *'CAMPAIGN_FAKE_TRANSPORT_VERDICT:'*'"transport":"injected-fake"'*'"cases":6'*'"pass":4'*'"fail":1'*'"skip":1'*'"class":"fail"'*) ;;
  *) fail "unexpected happy-path verdict: $result" ;;
esac
case "$result" in
  *'CAMPAIGN_VERDICT:'*) adversarial "a fake-transport run emitted the promotable CAMPAIGN_VERDICT marker: $result" ;;
esac

# =============================================================================
# 5. Manifest replay: pointing MANIFEST at a committed file reproduces the
#    exact same chunk assignment (a disagreement can be replayed).
# =============================================================================
replay_manifest="$tmp/replay.tsv"
(cd "$root" && CHUNKS=3 SEED=1 MANIFEST="$replay_manifest" "$DISTRIBUTE" manifest >/dev/null)
r1=$(cd "$root" && env "$FAKESEAM" CHUNKS=3 WORKERS="worker-a worker-b worker-c" \
  MANIFEST="$replay_manifest" OUT_DIR="$tmp/replay1" "$DISTRIBUTE" distribute)
r2=$(cd "$root" && env "$FAKESEAM" CHUNKS=3 WORKERS="worker-a worker-b worker-c" \
  MANIFEST="$replay_manifest" OUT_DIR="$tmp/replay2" "$DISTRIBUTE" distribute)
# Strip the OUT_DIR-dependent scratch-file paths before comparing — only the
# substantive per-test verdict must be reproducible, not the scratch layout.
strip_paths() { sed -e 's/"manifest":"[^"]*"/"manifest":X/' -e 's/"verdict":"[^"]*"/"verdict":X/'; }
diff "$tmp/replay1/verdict.tsv" "$tmp/replay2/verdict.tsv" >/dev/null \
  || fail "replaying the same committed manifest produced different per-test outcomes"
[ "$(printf '%s' "$r1" | strip_paths)" = "$(printf '%s' "$r2" | strip_paths)" ] \
  || fail "replaying the same committed manifest produced different verdict summaries"

# =============================================================================
# 6. Evidence invariant — unreachable worker fails the reduction, loudly.
# =============================================================================
if (cd "$root" && env "$FAKESEAM" FAKE_UNREACHABLE_WORKER=worker-b CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/unreachable" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/unreachable.err"); then
  adversarial "an unreachable worker was silently accepted as a passing chunk"
fi
grep -q 'worker unreachable\|dispatch failed' "$tmp/unreachable.err" \
  || adversarial "unreachable-worker failure did not name the cause"

# =============================================================================
# 7. Evidence invariant — a worker that runs but returns nothing for a
#    non-empty chunk is absence of evidence, not a pass.
# =============================================================================
if (cd "$root" && env "$FAKESEAM" FAKE_EMPTY_WORKER=worker-a CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/empty" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/empty.err"); then
  adversarial "an empty chunk result was silently accepted as a passing chunk"
fi

# =============================================================================
# 8. Evidence invariant — a chunk that drops one of its assigned cases must
#    fail the reduction (a missing case is not an implicit pass or skip).
# =============================================================================
if (cd "$root" && env "$FAKESEAM" FAKE_DROP_CASE=case-c CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/drop" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/drop.err"); then
  adversarial "a chunk silently dropping an assigned case was accepted"
fi

# =============================================================================
# 9. Evidence invariant — a chunk injecting a case it was not assigned must
#    also fail (foreign evidence is not a substitute for the real one, and
#    letting it through would let a chunk over-report and mask a drop
#    elsewhere via a coincidentally matching total).
# =============================================================================
if (cd "$root" && env "$FAKESEAM" FAKE_INJECT_FOREIGN_CASE=case-zz CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/foreign" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/foreign.err"); then
  adversarial "a chunk reporting a case outside its assigned set was accepted"
fi

# =============================================================================
# 10. Evidence invariant — a corrupted manifest that assigns one case to two
#     chunks must fail the reduction, even though each chunk faithfully
#     reports exactly its own (overlapping) assigned set.
# =============================================================================
dup_manifest="$tmp/dup-manifest.tsv"
cat >"$dup_manifest" <<'EOF'
0	case-a
0	case-b
1	case-c
1	case-d
2	case-e
2	case-f
2	case-a
EOF
if (cd "$root" && env "$FAKESEAM" CHUNKS=3 WORKERS="worker-a worker-b worker-c" \
  MANIFEST="$dup_manifest" OUT_DIR="$tmp/dup" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/dup.err"); then
  adversarial "a manifest assigning one case to two chunks was accepted"
fi
grep -q 'more than one chunk' "$tmp/dup.err" || adversarial "duplicate-case failure did not name the cause"

# =============================================================================
# 11. Evidence invariant — a corrupted manifest that omits a case entirely
#     must fail closed: no chunk can report evidence for a case it was never
#     told to run.
# =============================================================================
missing_manifest="$tmp/missing-manifest.tsv"
cat >"$missing_manifest" <<'EOF'
0	case-a
0	case-b
1	case-c
1	case-d
2	case-e
EOF
if (cd "$root" && env "$FAKESEAM" CHUNKS=3 WORKERS="worker-a worker-b worker-c" \
  MANIFEST="$missing_manifest" OUT_DIR="$tmp/missing" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/missing.err"); then
  adversarial "a manifest omitting a corpus case was accepted"
fi
grep -q 'missing evidence\|never reported' "$tmp/missing.err" \
  || adversarial "missing-case failure did not name the cause"

# =============================================================================
# 12. verify: serial and distributed agree exactly -> equivalence holds. Under
#     the fake seam the marker must be the logic-only one, never the
#     distributed-equivalence claim.
# =============================================================================
out=$(cd "$root" && env "$FAKESEAM" CHUNKS=3 SEED=1 WORKERS="worker-a worker-b worker-c" \
  OUT_DIR="$tmp/verify-ok" "$DISTRIBUTE" verify)
case "$out" in *'CAMPAIGN_LOGIC_EQUIVALENT:'*) ;; *) fail "verify did not report logic equivalence on matching runs: $out" ;; esac
case "$out" in
  *'CAMPAIGN_VERDICT_EQUIVALENT:'*) adversarial "a fake-transport verify claimed distributed verdict equivalence: $out" ;;
esac

# =============================================================================
# 13. verify — THE CORE DELIVERABLE: equal totals must not mask a set
#     mismatch. The fake executor swaps case-a and case-b's outcomes only in
#     the distributed path (a plausible bug: a worker mis-attributing which
#     case a result belongs to). Serial reports a=pass,b=fail; distributed
#     reports a=fail,b=pass. Both have 4 pass / 1 fail / 1 skip overall — a
#     count-only check would call this equivalent. It is not.
# =============================================================================
set +e
out=$(cd "$root" && env "$FAKESEAM" FAKE_SWAP_A=case-a FAKE_SWAP_B=case-b CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/verify-swap" "$DISTRIBUTE" verify 2>"$tmp/verify-swap.err")
rc=$?
set -e
if [ "$rc" -eq 0 ]; then
  adversarial "verify accepted a distributed run with the same totals but a different per-test outcome set"
fi
case "$out" in *'CAMPAIGN_VERDICT_MISMATCH:'*) ;; *) adversarial "verify mismatch did not report CAMPAIGN_VERDICT_MISMATCH: $out" ;; esac
grep -q 'case-a\|case-b' "$tmp/verify-swap.err" \
  || adversarial "verify mismatch diff did not name the disagreeing case(s)"

# =============================================================================
# 14. Zero-case chunks (CHUNKS > distinct cases) are handled without treating
#     an empty-because-nothing-assigned chunk as an evidence failure.
# =============================================================================
out=$(cd "$root" && env "$FAKESEAM" CHUNKS=8 SEED=3 \
  WORKERS="worker-a worker-b worker-c worker-d" \
  OUT_DIR="$tmp/overchunked" "$DISTRIBUTE" distribute)
case "$out" in *'"cases":6'*) ;; *) fail "over-chunked corpus lost or gained cases: $out" ;; esac

# =============================================================================
# ==== the k8s transport, exercised through a fake kubectl ====================
# =============================================================================
#
# The fake kubectl simulates a small peer cluster on disk: node objects with
# outpost host/backend labels under $FAKE_CLUSTER, applied Job manifests and a
# deletion log under $FAKE_K8S_STATE. "Running" a pod means deriving its logs
# from the recorded manifest (evidence header + canonical outcomes), so the
# entire manifest -> placement -> logs -> collection -> cleanup loop is
# exercised without any network or cluster.
fake_kubectl="$tmp/fake-kubectl.sh"
cat >"$fake_kubectl" <<'KFAKE'
#!/usr/bin/env bash
set -euo pipefail
state="${FAKE_K8S_STATE:?}"
cluster="${FAKE_CLUSTER:?}"
mkdir -p "$state/manifests"

verb="${1:-}"; shift || true

jsonpath_arg() {
  for a in "$@"; do
    case "$a" in jsonpath=*) printf '%s\n' "${a#jsonpath=}" ;; esac
  done
}

case "$verb" in
  get)
    kind="$1"; shift
    case "$kind" in
      node)
        node="$1"; shift
        jp="$(jsonpath_arg "$@")"
        [ -f "$cluster/$node" ] || { echo "Error: node \"$node\" not found" >&2; exit 1; }
        case "$jp" in
          '{.metadata.name}') printf '%s' "$node" ;;
          *host*) sed -n 's/^host=//p' "$cluster/$node" | tr -d '\n' ;;
          *backend*) sed -n 's/^backend=//p' "$cluster/$node" | tr -d '\n' ;;
          *) exit 1 ;;
        esac
        ;;
      job)
        job="$1"
        [ -f "$state/manifests/$job.yaml" ] || { echo "Error: job \"$job\" not found" >&2; exit 1; }
        printf 'uid-%s' "$job"
        ;;
      pods)
        job=""
        prev=""
        for a in "$@"; do
          if [ "$prev" = "-l" ]; then
            case "$a" in job-name=*) job="${a#job-name=}" ;; esac
          fi
          prev="$a"
        done
        [ -n "$job" ] || exit 1
        [ -f "$state/manifests/$job.yaml" ] || exit 1
        printf '%s\n' "${job}-pod0"
        ;;
      pod)
        pod="$1"; shift
        job="${pod%-pod0}"
        mf="$state/manifests/$job.yaml"
        [ -f "$mf" ] || exit 1
        jp="$(jsonpath_arg "$@")"
        case "$jp" in
          *ownerReferences*)
            if [ -n "${FAKE_FOREIGN_OWNER:-}" ]; then
              printf 'uid-somebody-else'
            else
              printf 'uid-%s' "$job"
            fi
            ;;
          *nodeName*)
            if [ -n "${FAKE_MISSCHEDULE_NODE:-}" ]; then
              printf '%s' "$FAKE_MISSCHEDULE_NODE"
            else
              awk '/^      nodeName: /{printf "%s", $2; exit}' "$mf"
            fi
            ;;
          *) exit 1 ;;
        esac
        ;;
      *) exit 1 ;;
    esac
    ;;
  apply)
    mf_content="$(cat)"
    job="$(printf '%s\n' "$mf_content" | awk '/^  name: /{print $2; exit}')"
    [ -n "$job" ] || { echo "fake-kubectl: manifest without a name" >&2; exit 1; }
    printf '%s\n' "$mf_content" >"$state/manifests/$job.yaml"
    echo "job.batch/$job created"
    ;;
  wait)
    if [ -n "${FAKE_K8S_WEDGED:-}" ]; then
      echo "error: timed out waiting for the condition" >&2
      exit 1
    fi
    ;;
  logs)
    pod="$1"
    job="${pod%-pod0}"
    mf="$state/manifests/$job.yaml"
    [ -f "$mf" ] || exit 1
    if [ -n "${FAKE_K8S_EMPTY_LOGS:-}" ]; then exit 0; fi
    envval() {
      grep -A1 "name: $1\$" "$mf" | sed -n 's/^ *value: "\(.*\)"$/\1/p' | sed -n 1p
    }
    node="$(awk '/^      nodeName: /{print $2; exit}' "$mf")"
    jobname="$(envval JOB_NAME)"
    chunk="$(envval CHUNK_ID)"
    worker="$(envval WORKER_ROLE)"
    suite="$(envval CAMPAIGN_SUITE)"
    chunk_cases="$(envval CHUNK_CASES)"
    printf 'CAMPAIGN_CHUNK_EVIDENCE:{"job":"%s","chunk":%s,"worker":"%s","node":"%s","suite":"%s"}\n' \
      "$jobname" "$chunk" "$worker" "$node" "$suite"
    for c in $chunk_cases; do
      printf '%s %s\n' "$c" "$(awk -v i="$c" '$1==i{print $2}' "$OUTCOME_MAP")"
    done
    ;;
  delete)
    kind="$1"; shift
    job="$1"
    printf '%s\n' "$job" >>"$state/deleted.log"
    ;;
  *)
    echo "fake-kubectl: unhandled verb '$verb'" >&2
    exit 1
    ;;
esac
KFAKE
chmod +x "$fake_kubectl"

# The simulated peer cluster: two honest workers, plus the dhnt#4 trap — one
# host presenting multiple virtual-backend nodes with the identical host label.
cluster="$tmp/cluster"
mkdir -p "$cluster"
printf 'host=host-a\nbackend=vk-a\n' >"$cluster/peer-node-1"
printf 'host=host-b\nbackend=vk-a\n' >"$cluster/peer-node-2"
printf 'host=host-t\nbackend=vk-t\n' >"$cluster/twin-1"
printf 'host=host-t\nbackend=vk-t\n' >"$cluster/twin-2"
printf 'host=host-t\nbackend=vk-u\n' >"$cluster/twin-2b"

export FAKE_CLUSTER="$cluster"
export CHUNK_IMAGE=dhnt.io/campaign-runner
export CHUNK_POD_CMD=run-chunk
PINS_OK="WORKER_NODES=worker-a=peer-node-1 worker-b=peer-node-2"

# =============================================================================
# 15. k8s happy path: one Job per chunk, pinned by node name to the two
#     distinct peer workers; results collected with job+node identity; every
#     created Job deleted; per-test verdict matches the canonical map; and —
#     because the kubectl is a fake — the verdict is marked fake-transport,
#     never CAMPAIGN_VERDICT.
# =============================================================================
st="$tmp/k8s-happy-state"
out=$(cd "$root" && env CAMPAIGN_TRANSPORT=k8s "CAMPAIGN_K8S_FAKE_KUBECTL=$fake_kubectl" \
  "FAKE_K8S_STATE=$st" "$PINS_OK" CHUNKS=2 SEED=1 WORKERS="worker-a worker-b" \
  OUT_DIR="$tmp/k8s-happy" "$DISTRIBUTE" distribute)
case "$out" in
  *'CAMPAIGN_FAKE_TRANSPORT_VERDICT:'*'"transport":"k8s-fake-kubectl"'*'"cases":6'*'"pass":4'*'"fail":1'*'"skip":1'*) ;;
  *) fail "unexpected k8s happy-path verdict: $out" ;;
esac
case "$out" in
  *'CAMPAIGN_VERDICT:'*) adversarial "a fake-kubectl k8s run emitted the promotable CAMPAIGN_VERDICT marker: $out" ;;
esac

created_n=$(ls "$st/manifests" | grep -c .)
[ "$created_n" -eq 2 ] || fail "expected 2 per-chunk Jobs, found $created_n manifests"

pinned_nodes=$(awk '/^      nodeName: /{print $2}' "$st/manifests/"*.yaml | sort -u)
[ "$pinned_nodes" = "peer-node-1
peer-node-2" ] || fail "chunk Jobs were not pinned to the two distinct peer nodes: $pinned_nodes"

for c in 0 1; do
  [ -s "$tmp/k8s-happy/chunk-$c.evidence.json" ] || fail "missing evidence sidecar for chunk $c"
  [ -s "$tmp/k8s-happy/chunk-$c.log" ] || fail "missing collected log for chunk $c"
  grep -q '"job":"cd-free-utils-sweep-c'"$c"'-' "$tmp/k8s-happy/chunk-$c.evidence.json" \
    || fail "evidence sidecar for chunk $c does not carry its Job identity"
  grep -q '"node":"peer-node-' "$tmp/k8s-happy/chunk-$c.evidence.json" \
    || fail "evidence sidecar for chunk $c does not carry its source node"
done

# every created Job must have been deleted (dispatch deletes; the parent's
# EXIT ledger sweep re-deletes idempotently)
while IFS= read -r job; do
  [ -n "$job" ] || continue
  grep -qx "$job" "$st/deleted.log" || fail "created Job $job was never deleted — workload leaked into the cluster"
done <"$tmp/k8s-happy/jobs.created"

diff <(sort "$outcomes") "$tmp/k8s-happy/verdict.tsv" >/dev/null \
  || fail "k8s-path per-test verdict does not match the canonical outcome map"

# =============================================================================
# 16. preflight — two roles pinning the SAME node is not distribution.
# =============================================================================
st="$tmp/k8s-dup-state"
set +e
out=$(cd "$root" && env CAMPAIGN_TRANSPORT=k8s "CAMPAIGN_K8S_FAKE_KUBECTL=$fake_kubectl" \
  "FAKE_K8S_STATE=$st" "WORKER_NODES=worker-a=peer-node-1 worker-b=peer-node-1" \
  CHUNKS=2 SEED=1 WORKERS="worker-a worker-b" OUT_DIR="$tmp/k8s-dup" "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -ne 0 ] || adversarial "two roles pinned to one node were accepted as distribution"
case "$out" in *'same node'*) ;; *) adversarial "duplicate-node refusal did not name the cause: $out" ;; esac
[ ! -d "$st/manifests" ] || [ -z "$(ls "$st/manifests" 2>/dev/null)" ] \
  || adversarial "preflight refusal happened only after Jobs were already created"

# =============================================================================
# 17. preflight — the dhnt#4 trap: two DISTINCT node names carrying the
#     identical outpost host label AND backend are one worker, not two.
#     With a backend discriminator the same host is accepted.
# =============================================================================
st="$tmp/k8s-twin-state"
set +e
out=$(cd "$root" && env CAMPAIGN_TRANSPORT=k8s "CAMPAIGN_K8S_FAKE_KUBECTL=$fake_kubectl" \
  "FAKE_K8S_STATE=$st" "WORKER_NODES=worker-a=twin-1 worker-b=twin-2" \
  CHUNKS=2 SEED=1 WORKERS="worker-a worker-b" OUT_DIR="$tmp/k8s-twin" "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -ne 0 ] || adversarial "two nodes with one host label + one backend were accepted as distinct workers"
case "$out" in *'SAME worker'*) ;; *) adversarial "host-label-trap refusal did not name the cause: $out" ;; esac

st="$tmp/k8s-twin-ok-state"
out=$(cd "$root" && env CAMPAIGN_TRANSPORT=k8s "CAMPAIGN_K8S_FAKE_KUBECTL=$fake_kubectl" \
  "FAKE_K8S_STATE=$st" "WORKER_NODES=worker-a=twin-1 worker-b=twin-2b" \
  CHUNKS=2 SEED=1 WORKERS="worker-a worker-b" OUT_DIR="$tmp/k8s-twin-ok" "$DISTRIBUTE" distribute) \
  || fail "same host with distinct backend discriminators was wrongly refused"
case "$out" in *'CAMPAIGN_FAKE_TRANSPORT_VERDICT:'*'"cases":6'*) ;; *) fail "backend-discriminated twin run produced no verdict: $out" ;; esac

# =============================================================================
# 18. preflight — fewer than two distinct workers is not distribution.
# =============================================================================
st="$tmp/k8s-one-state"
set +e
out=$(cd "$root" && env CAMPAIGN_TRANSPORT=k8s "CAMPAIGN_K8S_FAKE_KUBECTL=$fake_kubectl" \
  "FAKE_K8S_STATE=$st" "WORKER_NODES=worker-a=peer-node-1" \
  CHUNKS=2 SEED=1 WORKERS="worker-a" OUT_DIR="$tmp/k8s-one" "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -ne 0 ] || adversarial "a single-worker k8s run was accepted as distribution"
case "$out" in *'at least 2'*) ;; *) adversarial "single-worker refusal did not name the cause: $out" ;; esac

# =============================================================================
# 19. preflight — a pinned node that does not exist in the peer cluster.
# =============================================================================
st="$tmp/k8s-ghost-state"
set +e
out=$(cd "$root" && env CAMPAIGN_TRANSPORT=k8s "CAMPAIGN_K8S_FAKE_KUBECTL=$fake_kubectl" \
  "FAKE_K8S_STATE=$st" "WORKER_NODES=worker-a=peer-node-1 worker-b=ghost-node" \
  CHUNKS=2 SEED=1 WORKERS="worker-a worker-b" OUT_DIR="$tmp/k8s-ghost" "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -ne 0 ] || adversarial "a nonexistent pinned node was accepted"
case "$out" in *'does not exist'*) ;; *) adversarial "ghost-node refusal did not name the cause: $out" ;; esac

# =============================================================================
# 20. A Job that never completes (never scheduled / wedged) fails the run —
#     and is still cleaned up, not leaked.
# =============================================================================
st="$tmp/k8s-wedged-state"
set +e
out=$(cd "$root" && env CAMPAIGN_TRANSPORT=k8s "CAMPAIGN_K8S_FAKE_KUBECTL=$fake_kubectl" \
  "FAKE_K8S_STATE=$st" "$PINS_OK" FAKE_K8S_WEDGED=1 \
  CHUNKS=2 SEED=1 WORKERS="worker-a worker-b" OUT_DIR="$tmp/k8s-wedged" "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -ne 0 ] || adversarial "a Job that never completed was accepted as a passing chunk"
case "$out" in *'did not complete'*) ;; *) adversarial "wedged-Job failure did not name the cause: $out" ;; esac
[ -s "$tmp/k8s-wedged/jobs.created" ] || fail "wedged run recorded no created Jobs"
while IFS= read -r job; do
  [ -n "$job" ] || continue
  grep -qx "$job" "$st/deleted.log" \
    || adversarial "Job $job from the failed run was never deleted — workload leaked on the failure path"
done <"$tmp/k8s-wedged/jobs.created"

# =============================================================================
# 21. Placement evidence — a pod observed on a different node than its pin
#     is rejected: the pin did not hold, so the result is not attributable
#     to the worker it claims.
# =============================================================================
st="$tmp/k8s-rogue-state"
set +e
out=$(cd "$root" && env CAMPAIGN_TRANSPORT=k8s "CAMPAIGN_K8S_FAKE_KUBECTL=$fake_kubectl" \
  "FAKE_K8S_STATE=$st" "$PINS_OK" FAKE_MISSCHEDULE_NODE=rogue-node \
  CHUNKS=2 SEED=1 WORKERS="worker-a worker-b" OUT_DIR="$tmp/k8s-rogue" "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -ne 0 ] || adversarial "a pod that ran on an unpinned node was accepted"
case "$out" in *'pin did not hold'*|*rogue-node*) ;; *) adversarial "misplacement failure did not name the node: $out" ;; esac

# =============================================================================
# 22. Identity evidence — a pod not owned by the chunk's Job is foreign
#     evidence and must be rejected.
# =============================================================================
st="$tmp/k8s-foreign-state"
set +e
out=$(cd "$root" && env CAMPAIGN_TRANSPORT=k8s "CAMPAIGN_K8S_FAKE_KUBECTL=$fake_kubectl" \
  "FAKE_K8S_STATE=$st" "$PINS_OK" FAKE_FOREIGN_OWNER=1 \
  CHUNKS=2 SEED=1 WORKERS="worker-a worker-b" OUT_DIR="$tmp/k8s-foreignpod" "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -ne 0 ] || adversarial "a pod owned by a different Job was accepted as chunk evidence"
case "$out" in *'not owned by Job'*) ;; *) adversarial "foreign-owner failure did not name the cause: $out" ;; esac

# =============================================================================
# 23. Lost/empty logs — a chunk whose pod produced no evidence header is a
#     lost log: missing evidence, never a pass.
# =============================================================================
st="$tmp/k8s-nolog-state"
set +e
out=$(cd "$root" && env CAMPAIGN_TRANSPORT=k8s "CAMPAIGN_K8S_FAKE_KUBECTL=$fake_kubectl" \
  "FAKE_K8S_STATE=$st" "$PINS_OK" FAKE_K8S_EMPTY_LOGS=1 \
  CHUNKS=2 SEED=1 WORKERS="worker-a worker-b" OUT_DIR="$tmp/k8s-nolog" "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -ne 0 ] || adversarial "a chunk with no evidence header in its logs was accepted"
case "$out" in *'CAMPAIGN_CHUNK_EVIDENCE'*) ;; *) adversarial "lost-log failure did not name the missing evidence header: $out" ;; esac

# =============================================================================
# 24. The real path's cluster-selection guards (no fake kubectl injected):
#     the transport is peer-only and the D3 profile is authoritative.
# =============================================================================
# a. an explicit cloudbox profile is refused — no cloudbox dependency in the
#    peer execution path.
set +e
out=$(cd "$root" && env -u CAMPAIGN_K8S_FAKE_KUBECTL DKS_PROFILE=cloudbox \
  WORKERS="worker-a worker-b" "$PINS_OK" "$K8S" preflight 2>&1)
rc=$?
set -e
[ "$rc" -eq 8 ] || fail "DKS_PROFILE=cloudbox was not refused by the peer transport (got $rc): $out"
case "$out" in *'peer execution only'*) ;; *) fail "cloudbox refusal did not name the cause: $out" ;; esac

# b. peer profile with a missing kubeconfig fails loudly through
#    dks-profile.sh — never a silent fallback. An ambient $KUBECTL must NOT
#    rescue it: the profile is authoritative over the environment.
set +e
out=$(cd "$root" && env -u CAMPAIGN_K8S_FAKE_KUBECTL DKS_PROFILE=peer \
  "DKS_PEER_KUBECONFIG=$tmp/absent-kubeconfig.yaml" "KUBECTL=echo ambient-kubectl" \
  WORKERS="worker-a worker-b" "$PINS_OK" "$K8S" preflight 2>&1)
rc=$?
set -e
[ "$rc" -eq 9 ] || fail "missing peer kubeconfig did not fail loudly through dks-profile (got $rc): $out"
case "$out" in *'refusing to fall back to cloudbox'*) ;; *) fail "peer kubeconfig failure did not come from dks-profile: $out" ;; esac
case "$out" in *ambient-kubectl*) fail "an ambient \$KUBECTL overrode the peer profile: $out" ;; *) ;; esac

[ "$adversarial_fail" -eq 0 ] || exit 1
echo "campaign-distribute verdict equivalence + peer-DKS transport: PASS"
