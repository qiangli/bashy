#!/usr/bin/env bash
# Real peer-DKS transport for scripts/campaign-distribute.sh: one Kubernetes
# Job per chunk, pinned to a distinct peer worker NODE, evidence-collected,
# cleaned up. Never invoked directly by users — campaign-distribute.sh calls
# the subcommands below when CAMPAIGN_TRANSPORT=k8s.
#
# THE CERTIFICATION LINE (defense in depth — campaign-distribute.sh already
# refuses first): certification runs on ONE unchunked, exclusive SUT. This
# script exists only to distribute, so MODE=cert refuses here too, before
# anything else.
#
# Cluster selection is bashy#27/D3's job, not this script's: the peer
# kubeconfig profile is consumed exclusively through dks-profile.sh's
# dks_resolve_kubectl inside campaign_k8s_resolve_kubectl() below — the
# single integration shim. This transport is PEER-ONLY: DKS_PROFILE=peer is
# forced, an explicit cloudbox profile is refused, and an ambient $KUBECTL is
# deliberately NOT honored (the profile is authoritative; a test injects a
# kubectl through CAMPAIGN_K8S_FAKE_KUBECTL, never through $KUBECTL).
#
# Worker identity and the dhnt#4 trap: `outpost.dhnt.io/host` labels a HOST,
# not a node — a host running two virtual backends presents TWO nodes with
# the identical host label, so pinning by that label can land two "distinct"
# chunks on one worker. This script pins by NODE NAME (spec.nodeName — no
# scheduler, no label matching) and `preflight` independently verifies that
# the pinned nodes exist and resolve to distinct workers: distinct node
# names AND distinct (host-label, backend-label) identity tuples. Two nodes
# sharing a host label are the same worker unless a backend discriminator
# separates them.
#
# Subcommands:
#   preflight                       verify the WORKERS -> node pinning is real
#                                   distribution before any Job is created
#   dispatch-chunk WORKER CASES_FILE OUT_FILE CHUNK_ID EVIDENCE_DIR
#                                   create, await, collect, and delete one
#                                   per-chunk Job
#   cleanup LEDGER                  delete every Job named in the ledger file
#                                   (idempotent; parent runs it on EXIT)
#
# Required env (preflight/dispatch-chunk):
#   SUITE          campaign/free-suite label (also stamped into evidence)
#   WORKERS        space-separated peer-worker ROLE names (hosts are named by
#                  role, never by hostname/user)
#   WORKER_NODES   space-separated role=node pins, e.g.
#                  "worker-a=peer-node-1 worker-b=peer-node-2"
#   CHUNK_IMAGE    container image providing the chunk runner
#   CHUNK_POD_CMD  command inside the image, invoked as
#                  `$CHUNK_POD_CMD <cases_file> <worker_role>`; must print one
#                  `<test_id> <outcome>` line per case (same contract as
#                  RUN_CHUNK_CMD)
# Optional:
#   NS             namespace (default: default)
#   CHUNK_TIMEOUT  seconds to wait for one chunk Job (default: 1800)
#   TTL            ttlSecondsAfterFinished backstop (default: 3600) — the
#                  primary cleanup is the explicit delete, not the TTL
set -euo pipefail

MODE="${MODE:-campaign}"
if [ "$MODE" = cert ]; then
  echo "campaign-distribute-k8s: REFUSED — MODE=cert may never distribute or chunk a run." >&2
  echo "campaign-distribute-k8s: certification runs on one unchunked, exclusive SUT." >&2
  exit 77
fi

root=$(cd "$(dirname "$0")" && pwd)

NS="${NS:-default}"
CHUNK_TIMEOUT="${CHUNK_TIMEOUT:-1800}"
TTL="${TTL:-3600}"

# --- THE D3 INTEGRATION SHIM -------------------------------------------------
#
# The single place this transport learns how to reach the peer cluster.
# Everything below consumes $KUBECTL opaquely; if bashy#27/D3 changes the
# profile interface, this one function body is the entire integration.
#
# CAMPAIGN_K8S_FAKE_KUBECTL is the unit-injection seam for the deterministic
# gate: it replaces the kubectl transport with a test-supplied fake. It is
# never a real execution mode — campaign-distribute.sh refuses to emit a
# CAMPAIGN_VERDICT (the only promotable marker) while it is set.
campaign_k8s_resolve_kubectl() {
  if [ -n "${CAMPAIGN_K8S_FAKE_KUBECTL:-}" ]; then
    echo "campaign-distribute-k8s: FAKE kubectl injected — logic check only; this run is NOT distributed execution and its output is not a distributed result" >&2
    printf '%s\n' "$CAMPAIGN_K8S_FAKE_KUBECTL"
    return 0
  fi
  case "${DKS_PROFILE:-peer}" in
    peer) ;;
    *)
      echo "campaign-distribute-k8s: REFUSED — peer execution only; DKS_PROFILE='${DKS_PROFILE}' has no place in the campaign transport (no cloudbox dependency in the peer path)" >&2
      exit 8
      ;;
  esac
  export DKS_PROFILE=peer
  # Peer profile is authoritative over any ambient $KUBECTL: resolve through
  # the D3 profile unconditionally instead of the historical
  # `[ -z "$KUBECTL" ]` dance, so an inherited cloudbox KUBECTL can never
  # silently steal a peer run.
  . "$root/dks-profile.sh"
  dks_resolve_kubectl "kubectl"
}

KUBECTL="$(campaign_k8s_resolve_kubectl)"

# --- role -> node pin lookup -------------------------------------------------
campaign_k8s_node_for() {
  role="$1"
  for pair in ${WORKER_NODES:-}; do
    case "$pair" in
      "$role"=*) printf '%s\n' "${pair#*=}"; return 0 ;;
    esac
  done
  echo "campaign-distribute-k8s: no node pin for worker role '$role' in WORKER_NODES" >&2
  return 1
}

campaign_k8s_check_token() {
  # Everything interpolated into a Job manifest must be a plain token —
  # refusing metacharacters keeps YAML injection structurally impossible.
  val="$1" what="$2"
  case "$val" in
    *[!A-Za-z0-9._:+/-]*|'')
      echo "campaign-distribute-k8s: $what '$val' contains characters outside [A-Za-z0-9._:+/-] — refused" >&2
      exit 2
      ;;
  esac
}

# --- preflight: pinning must be real distribution ----------------------------
campaign_k8s_preflight() {
  : "${WORKERS:?set WORKERS to space-separated peer-worker role names}"
  : "${WORKER_NODES:?set WORKER_NODES to space-separated role=node pins}"
  : "${CHUNK_IMAGE:?set CHUNK_IMAGE to the chunk-runner container image}"
  : "${CHUNK_POD_CMD:?set CHUNK_POD_CMD to the in-pod chunk runner}"

  nodes=""
  for role in $WORKERS; do
    node="$(campaign_k8s_node_for "$role")" || exit 10
    campaign_k8s_check_token "$node" "node name"
    campaign_k8s_check_token "$role" "worker role"
    nodes="$nodes$node
"
  done

  dup="$(printf '%s' "$nodes" | sort | uniq -d)"
  if [ -n "$dup" ]; then
    echo "campaign-distribute-k8s: REFUSED — two worker roles pin the same node ($(printf '%s' "$dup" | tr '\n' ' ')). Two chunks on one worker is not distribution." >&2
    exit 11
  fi

  distinct_n="$(printf '%s' "$nodes" | sort -u | grep -c .)"
  if [ "$distinct_n" -lt 2 ]; then
    echo "campaign-distribute-k8s: REFUSED — only $distinct_n distinct worker node(s) pinned; distribution requires at least 2." >&2
    exit 12
  fi

  # Verify each pinned node actually exists, and that the pins resolve to
  # distinct WORKERS, not just distinct node objects. outpost.dhnt.io/host
  # labels a HOST, not a node: two virtual-backend nodes on one host carry
  # the identical host label and are the same worker unless the backend
  # label discriminates them.
  identities=""
  for role in $WORKERS; do
    node="$(campaign_k8s_node_for "$role")"
    got="$($KUBECTL get node "$node" -o jsonpath='{.metadata.name}' 2>/dev/null)" || {
      echo "campaign-distribute-k8s: REFUSED — worker role '$role' pins node '$node' which does not exist in the peer cluster" >&2
      exit 13
    }
    [ "$got" = "$node" ] || {
      echo "campaign-distribute-k8s: REFUSED — node lookup for '$node' returned '$got'" >&2
      exit 13
    }
    host="$($KUBECTL get node "$node" -o jsonpath='{.metadata.labels.outpost\.dhnt\.io/host}' 2>/dev/null || true)"
    backend="$($KUBECTL get node "$node" -o jsonpath='{.metadata.labels.outpost\.dhnt\.io/backend}' 2>/dev/null || true)"
    if [ -n "$host" ]; then
      identity="$host|$backend"
    else
      # No host label: the node name is the only identity we have.
      identity="node:$node"
    fi
    identities="$identities$identity
"
    echo "campaign-distribute-k8s: pin role=$role node=$node host=${host:-<none>} backend=${backend:-<none>}" >&2
  done

  dup_id="$(printf '%s' "$identities" | sort | uniq -d)"
  if [ -n "$dup_id" ]; then
    echo "campaign-distribute-k8s: REFUSED — distinct node names resolve to the SAME worker (identical host+backend labels: $(printf '%s' "$dup_id" | tr '\n' ' ')). outpost.dhnt.io/host labels a host, not a node — this pinning is not distribution." >&2
    exit 14
  fi

  echo "CAMPAIGN_K8S_PREFLIGHT_OK: $distinct_n distinct peer worker nodes pinned" >&2
}

# --- per-chunk Job manifest ---------------------------------------------------
campaign_k8s_manifest() {
  job="$1" node="$2" worker="$3" chunk_id="$4" cases="$5"
  cat <<YAML
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job}
  namespace: ${NS}
  labels:
    app: ${job}
    dhnt.io/lane: campaign-distribute
spec:
  completions: 1
  parallelism: 1
  backoffLimit: 0
  ttlSecondsAfterFinished: ${TTL}
  template:
    metadata:
      labels:
        app: ${job}
        dhnt.io/lane: campaign-distribute
    spec:
      restartPolicy: Never
      nodeName: ${node}
      tolerations:
        - key: virtual-kubelet.io/provider
          operator: Equal
          value: outpost
          effect: NoSchedule
      containers:
        - name: chunk
          image: ${CHUNK_IMAGE}
          env:
            - name: JOB_NAME
              value: "${job}"
            - name: CHUNK_ID
              value: "${chunk_id}"
            - name: WORKER_ROLE
              value: "${worker}"
            - name: CAMPAIGN_SUITE
              value: "${SUITE}"
            - name: CHUNK_CASES
              value: "${cases}"
            - name: CHUNK_POD_CMD
              value: "${CHUNK_POD_CMD}"
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
          command: ["/bin/sh", "-c"]
          args:
            - |
              set -eu
              printf 'CAMPAIGN_CHUNK_EVIDENCE:{"job":"%s","chunk":%s,"worker":"%s","node":"%s","suite":"%s"}\n' \
                "\$JOB_NAME" "\$CHUNK_ID" "\$WORKER_ROLE" "\$NODE_NAME" "\$CAMPAIGN_SUITE"
              tmp=\$(mktemp)
              for c in \$CHUNK_CASES; do printf '%s\n' "\$c"; done >"\$tmp"
              exec \$CHUNK_POD_CMD "\$tmp" "\$WORKER_ROLE"
YAML
}

# --- dispatch-chunk: create, await, collect, delete ---------------------------
campaign_k8s_dispatch_chunk() {
  worker="$1" chunk_cases_file="$2" out_file="$3" chunk_id="$4" evidence_dir="$5"
  : "${SUITE:?set SUITE to the campaign/free-suite label}"
  : "${CHUNK_IMAGE:?set CHUNK_IMAGE to the chunk-runner container image}"
  : "${CHUNK_POD_CMD:?set CHUNK_POD_CMD to the in-pod chunk runner}"
  campaign_k8s_check_token "$SUITE" "SUITE"
  campaign_k8s_check_token "$worker" "worker role"

  node="$(campaign_k8s_node_for "$worker")" || exit 10
  campaign_k8s_check_token "$node" "node name"

  cases=""
  while IFS= read -r tc; do
    [ -n "$tc" ] || continue
    campaign_k8s_check_token "$tc" "test-case id"
    cases="$cases $tc"
  done <"$chunk_cases_file"
  cases="${cases# }"
  [ -n "$cases" ] || {
    echo "campaign-distribute-k8s: chunk $chunk_id has no cases — refusing to create an empty Job" >&2
    exit 2
  }

  suite_slug="$(printf '%s' "$SUITE" | tr 'A-Z' 'a-z' | sed -e 's/[^a-z0-9-]/-/g' | cut -c1-20)"
  job="cd-${suite_slug}-c${chunk_id}-$(date +%s)-$$"

  mkdir -p "$evidence_dir"
  ledger="$evidence_dir/jobs.created"

  # Record intent BEFORE apply, delete on every exit path: a Job must not
  # outlive its collection even when this process dies mid-flight. The
  # parent's EXIT-trap cleanup of the ledger is the second line of defense,
  # ttlSecondsAfterFinished the third.
  printf '%s\n' "$job" >>"$ledger"
  trap '$KUBECTL delete job "$job" -n "$NS" --ignore-not-found --wait=false >/dev/null 2>&1 || true' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM

  campaign_k8s_manifest "$job" "$node" "$worker" "$chunk_id" "$cases" \
    | $KUBECTL apply -f - >/dev/null

  if ! $KUBECTL wait --for=condition=complete "--timeout=${CHUNK_TIMEOUT}s" "job/$job" -n "$NS" >/dev/null 2>&1; then
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id worker=$worker node=$node — Job $job did not complete within ${CHUNK_TIMEOUT}s (never scheduled, wedged, or failed); worker unreachable is not a pass" >&2
    exit 3
  fi

  job_uid="$($KUBECTL get job "$job" -n "$NS" -o jsonpath='{.metadata.uid}')"
  [ -n "$job_uid" ] || {
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — Job $job has no uid; identity cannot be established" >&2
    exit 3
  }

  pods="$($KUBECTL get pods -n "$NS" -l "job-name=$job" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')"
  pod_n="$(printf '%s' "$pods" | grep -c . || true)"
  if [ "$pod_n" -ne 1 ]; then
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — expected exactly 1 pod for Job $job, found $pod_n; ambiguous evidence is not evidence" >&2
    exit 3
  fi
  pod="$(printf '%s' "$pods" | head -n1)"

  owner_uid="$($KUBECTL get pod "$pod" -n "$NS" -o jsonpath='{.metadata.ownerReferences[0].uid}')"
  if [ "$owner_uid" != "$job_uid" ]; then
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — pod $pod is not owned by Job $job (owner uid '$owner_uid' != job uid '$job_uid'); foreign evidence rejected" >&2
    exit 5
  fi

  observed_node="$($KUBECTL get pod "$pod" -n "$NS" -o jsonpath='{.spec.nodeName}')"
  if [ "$observed_node" != "$node" ]; then
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — pinned node '$node' but pod $pod ran on '$observed_node'; the pin did not hold, placement evidence rejected" >&2
    exit 5
  fi

  log_file="$evidence_dir/chunk-$chunk_id.log"
  $KUBECTL logs "$pod" -n "$NS" >"$log_file" || {
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — could not collect logs from pod $pod; a lost log is missing evidence, not a pass" >&2
    exit 4
  }

  header="$(sed -n '1p' "$log_file")"
  case "$header" in
    CAMPAIGN_CHUNK_EVIDENCE:*) ;;
    *)
      echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — pod $pod logs carry no CAMPAIGN_CHUNK_EVIDENCE header; unattributed output is not evidence" >&2
      exit 4
      ;;
  esac
  hdr_job="$(printf '%s' "$header" | sed -n 's/.*"job":"\([^"]*\)".*/\1/p')"
  hdr_chunk="$(printf '%s' "$header" | sed -n 's/.*"chunk":\([0-9][0-9]*\).*/\1/p')"
  hdr_node="$(printf '%s' "$header" | sed -n 's/.*"node":"\([^"]*\)".*/\1/p')"
  if [ "$hdr_job" != "$job" ] || [ "$hdr_chunk" != "$chunk_id" ] || [ "$hdr_node" != "$observed_node" ]; then
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — evidence header (job=$hdr_job chunk=$hdr_chunk node=$hdr_node) does not match Job $job chunk $chunk_id node $observed_node" >&2
    exit 5
  fi

  sed '1d' "$log_file" >"$out_file"

  printf '%s\n' "$observed_node" >"$evidence_dir/chunk-$chunk_id.node"
  printf '{"schema":"bashy.campaign.evidence/v1","suite":"%s","chunk":%s,"worker":"%s","job":"%s","job_uid":"%s","pod":"%s","node":"%s","log":"%s"}\n' \
    "$SUITE" "$chunk_id" "$worker" "$job" "$job_uid" "$pod" "$observed_node" "$log_file" \
    >"$evidence_dir/chunk-$chunk_id.evidence.json"

  $KUBECTL delete job "$job" -n "$NS" --ignore-not-found --wait=false >/dev/null
  trap - EXIT
}

# --- cleanup: delete every Job the ledger records ------------------------------
campaign_k8s_cleanup() {
  ledger="$1"
  [ -f "$ledger" ] || return 0
  while IFS= read -r job; do
    [ -n "$job" ] || continue
    $KUBECTL delete job "$job" -n "$NS" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  done <"$ledger"
}

subcmd="${1:-}"
case "$subcmd" in
  preflight)
    campaign_k8s_preflight
    ;;
  dispatch-chunk)
    shift
    [ $# -eq 5 ] || {
      echo "campaign-distribute-k8s: dispatch-chunk needs WORKER CASES_FILE OUT_FILE CHUNK_ID EVIDENCE_DIR" >&2
      exit 2
    }
    campaign_k8s_dispatch_chunk "$@"
    ;;
  cleanup)
    shift
    [ $# -eq 1 ] || {
      echo "campaign-distribute-k8s: cleanup needs LEDGER" >&2
      exit 2
    }
    campaign_k8s_cleanup "$1"
    ;;
  *)
    echo "campaign-distribute-k8s: unknown subcommand '$subcmd' (want: preflight|dispatch-chunk|cleanup)" >&2
    exit 2
    ;;
esac
