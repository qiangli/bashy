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
# forced, an explicit cloudbox profile is refused, an ambient $KUBECONFIG is
# refused outright, and an ambient $KUBECTL is deliberately NOT honored (the
# profile is authoritative; a test injects a kubectl through
# CAMPAIGN_K8S_FAKE_KUBECTL, never through $KUBECTL).
#
# PEER CLUSTER IDENTITY IS FAIL-CLOSED. Reaching *a* cluster is not evidence
# of reaching the RIGHT one, and "whatever context happened to be ambient"
# is precisely the cloudbox dependency this transport must not have. So
# before any Job is created, the operator must have pinned the peer control
# plane by CA fingerprint in CAMPAIGN_PEER_CA_SHA256, and the kubeconfig
# this transport would actually use must present exactly that CA. An absent
# pin is a REFUSAL, not a default — see campaign_k8s_verify_cluster_identity.
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
#                  "worker-a=peer-node-1 worker-b=peer-node-2". When set it is a
#                  fully manual, backward-compatible OVERRIDE: every role is
#                  hard-pinned and the resource-aware selector is never consulted.
#   CHUNK_IMAGE    container image providing the chunk runner
#   CHUNK_POD_CMD  command inside the image, invoked as
#                  `$CHUNK_POD_CMD <cases_file> <worker_role>`; must print one
#                  `<test_id> <outcome>` line per case (same contract as
#                  RUN_CHUNK_CMD)
#   CAMPAIGN_PEER_CA_SHA256
#                  the operator's pin for the peer control plane: the SHA256,
#                  in hex (an optional "sha256:" prefix and any ':' separators
#                  are stripped; case-insensitive), of the DECODED
#                  certificate-authority bytes of the cluster the peer
#                  kubeconfig's current-context selects. Compute it once, on
#                  the peer control plane, with:
#
#                    kubectl --kubeconfig "$DKS_PEER_KUBECONFIG" config view \
#                      --raw --minify \
#                      -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' \
#                      | base64 -d | sha256sum | cut -d' ' -f1
#
#                  There is no default and no TOFU: absence refuses.
# Optional:
#   DKS_PEER_KUBECONFIG
#                  peer kubeconfig path (dks-profile.sh's variable; default
#                  $HOME/.kube/outpost-control-plane/k3s.yaml). The identity
#                  check reads THIS file — the same one the resolved $KUBECTL
#                  will use — so the pin binds the transport, not a bystander.
#   NS             namespace (default: default)
#   CHUNK_TIMEOUT  seconds to wait for one chunk Job (default: 1800)
#   TTL            ttlSecondsAfterFinished backstop (default: 3600) — the
#                  primary cleanup is the explicit delete, not the TTL
#
# AUTO PLACEMENT (opt-in; peer campaign / free-suite shards). When WORKER_NODES
# is UNSET and CAMPAIGN_AUTO_PLACEMENT is truthy, `preflight` resolves each
# logical WORKERS role to a DISTINCT eligible node with scripts/dks-select-node.sh
# (the resource-aware selector — max(live metrics, reserved requests), Dragon /
# control-plane penalty, novicortex/novidesign preference). Each resolution feeds
# the nodes already chosen through DKS_SELECT_EXCLUDE_NODES so one point-in-time
# preflight cannot pick a node twice, and the resolved role=node map is persisted
# to a run-local, non-secret file ($CAMPAIGN_K8S_PLACEMENT_FILE, set by
# campaign-distribute.sh to $OUT_DIR/placement.env). dispatch-chunk READS that
# file — it never re-runs the selector, so the placement is decided exactly once
# and cannot drift or be recomputed concurrently. Auto placement is fail-closed:
# if the selector finds no eligible node for any role (insufficient / stale /
# unavailable capacity), preflight refuses before any Job is created.
#   CAMPAIGN_AUTO_PLACEMENT   1|true|yes|on enables auto placement (default off).
#   CAMPAIGN_SHARD_OS         kubernetes.io/os for auto selection (default linux).
#   CAMPAIGN_SHARD_BACKEND    outpost.dhnt.io/backend (default vk-podman).
#   CAMPAIGN_SHARD_ARCH       kubernetes.io/arch (default: any).
#   CAMPAIGN_SHARD_CPU        per-shard CPU request as a k8s quantity. Sizes the
#                             selection AND becomes the Job's resources.requests.cpu
#                             (default in auto mode: 1; unset in manual mode).
#   CAMPAIGN_SHARD_MEM        per-shard memory request (default in auto mode: 1Gi;
#                             unset in manual mode). The emitted Job carries these
#                             exact requests so Kubernetes enforces the reservation
#                             and it is visible to the NEXT selection as used.
set -euo pipefail

# Unset (no colon) so a truly-absent MODE still defaults to the normal
# execution mode, but an explicitly empty MODE="" is preserved as empty and
# falls through to the allowlist check below, which refuses it.
MODE="${MODE-campaign}"

# Allowlist, not denylist — defense in depth, mirroring campaign-distribute.sh:
# refuse unless MODE, after normalizing whitespace and case, is exactly
# "campaign". An unrecognized value is treated as cert-like and refused,
# never as campaign-like and allowed.
mode_norm="$(printf '%s' "$MODE" | tr '[:upper:]' '[:lower:]' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
if [ "$mode_norm" != campaign ]; then
  echo "campaign-distribute-k8s: REFUSED — MODE must be exactly 'campaign' (whitespace/case-insensitive); got MODE='$MODE'." >&2
  echo "campaign-distribute-k8s: certification runs on one unchunked, exclusive SUT." >&2
  exit 77
fi

root=$(cd "$(dirname "$0")" && pwd)

# An ambient $KUBECONFIG is ambient cluster context, and this transport must
# never inherit one: the peer profile is the only authority for which cluster
# a campaign is distributed onto. Refusing beats silently overriding it,
# because an operator who exported a cloudbox KUBECONFIG believes that is
# where their commands are going — and a refusal is the only outcome that
# tells them it is not.
if [ -n "${KUBECONFIG:-}" ]; then
  echo "campaign-distribute-k8s: REFUSED — an ambient \$KUBECONFIG is set ('$KUBECONFIG'). This transport is peer-only and the DKS peer profile is authoritative; an inherited (cloudbox or other) cluster context must never steer a campaign distribution." >&2
  echo "campaign-distribute-k8s: unset KUBECONFIG, select the peer control plane with DKS_PEER_KUBECONFIG, and pin it with CAMPAIGN_PEER_CA_SHA256." >&2
  exit 8
fi

NS="${NS:-default}"
CHUNK_TIMEOUT="${CHUNK_TIMEOUT:-1800}"
TTL="${TTL:-3600}"

# --- placement mode + per-shard resource requests ----------------------------
#
# Manual mode (WORKER_NODES set) is the fully backward-compatible override.
# Auto mode is opt-in and only when WORKER_NODES is UNSET, so an explicit pin
# always wins over the selector.
campaign_k8s_auto_mode() {
  [ -z "${WORKER_NODES:-}" ] || return 1
  case "${CAMPAIGN_AUTO_PLACEMENT:-}" in
    1 | true | yes | on | TRUE | YES | ON) return 0 ;;
    *) return 1 ;;
  esac
}

# The per-shard requests, computed ONCE so the value that sizes auto selection
# is byte-for-byte the value stamped into the Job's resources.requests. In
# manual mode they default to empty (no resources block -> the manifest is
# unchanged from before), unless the operator sets them explicitly.
if [ -n "${CAMPAIGN_SHARD_CPU:-}" ]; then
  REQ_CPU="$CAMPAIGN_SHARD_CPU"
elif campaign_k8s_auto_mode; then
  REQ_CPU="${CAMPAIGN_AUTO_DEFAULT_CPU:-1}"
else
  REQ_CPU=""
fi
if [ -n "${CAMPAIGN_SHARD_MEM:-}" ]; then
  REQ_MEM="$CAMPAIGN_SHARD_MEM"
elif campaign_k8s_auto_mode; then
  REQ_MEM="${CAMPAIGN_AUTO_DEFAULT_MEM:-1Gi}"
else
  REQ_MEM=""
fi

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
  . "$root/dks-profile.sh"
  if [ -n "${CAMPAIGN_K8S_FAKE_KUBECTL:-}" ]; then
    echo "campaign-distribute-k8s: FAKE kubectl injected — logic check only; this run is NOT distributed execution and its output is not a distributed result" >&2
    DKS_KUBECTL_CMD=("$CAMPAIGN_K8S_FAKE_KUBECTL")
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
  # The campaign transport deliberately ignores an ambient legacy KUBECTL
  # override: peer profile selection is authoritative here.
  unset KUBECTL
  dks_kubectl_init "kubectl"
}

campaign_k8s_resolve_kubectl

# --- FAIL-CLOSED PEER CLUSTER IDENTITY ---------------------------------------
#
# `preflight` proves the pinned NODES are two real, distinct workers. It says
# nothing about WHICH CLUSTER those nodes live in — a kubeconfig pointing at
# the wrong control plane answers every one of those checks happily, and the
# run distributes a campaign onto a cluster nobody chose. The identity gate
# below closes that: the operator pins the peer control plane's CA by SHA256,
# and this transport refuses to create a single Job unless the kubeconfig it
# would use presents exactly that CA.
#
# It is deliberately unconditional — it runs on the fake-kubectl seam too, so
# the deterministic gate exercises the same code the real path does, and no
# execution mode exists in which a campaign is distributed onto an unverified
# cluster.
#
# Nothing read out of the kubeconfig is ever printed: not the CA bytes, not
# the server URL, not a token or client key. The only values that reach a log
# are the two digests, and a CA certificate fingerprint is a public value by
# construction (that is the whole point of pinning one).

# Decode base64 on stdin. GNU, BSD and bashy's coreutils spell the flag
# differently; probe against /dev/null so the real stdin is untouched.
campaign_k8s_b64_decode() {
  if base64 --decode </dev/null >/dev/null 2>&1; then
    base64 --decode
  elif base64 -d </dev/null >/dev/null 2>&1; then
    base64 -d
  elif base64 -D </dev/null >/dev/null 2>&1; then
    base64 -D
  else
    openssl base64 -d
  fi
}

# SHA256 of stdin, as bare lowercase hex.
campaign_k8s_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  else
    openssl dgst -sha256 | awk '{print $NF}'
  fi
}

# Print "<data|file> <value>" for the certificate-authority of the cluster the
# kubeconfig's current-context selects — i.e. the MINIFIED view of the file,
# not "some cluster somewhere in it". A kubeconfig routinely carries several
# clusters (peer AND cloudbox); fingerprinting the wrong one would be a pin
# that passes while pointing somewhere else entirely.
#
# Exit codes name what is missing so the caller can refuse specifically:
#   2 no current-context   3 no/ambiguous context   4 no/ambiguous cluster
#   5 the selected cluster carries no certificate-authority at all
campaign_k8s_kubeconfig_ca_ref() {
  awk '
    function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
    function unq(s) { if (s ~ /^".*"$/) s = substr(s, 2, length(s) - 2); return s }
    /^[ \t]*#/ { next }
    {
      raw = $0
      # A key at column 0 opens a new top-level section and resets the
      # per-section list-item counter.
      if (raw ~ /^[A-Za-z][A-Za-z0-9_.-]*[ \t]*:/) {
        p = index(raw, ":")
        section = trim(substr(raw, 1, p - 1))
        if (section == "current-context") cc = unq(trim(substr(raw, p + 1)))
        item = 0
        next
      }
      if (section != "clusters" && section != "contexts") next
      body = raw
      if (body ~ /^[ \t]*-[ \t]*$/) { item++; next }
      if (body ~ /^[ \t]*-[ \t]/) { item++; sub(/^[ \t]*-[ \t]+/, "", body) }
      body = trim(body)
      if (body !~ /^[A-Za-z][A-Za-z0-9_.\/-]*[ \t]*:/) next
      p = index(body, ":")
      k = trim(substr(body, 1, p - 1))
      v = unq(trim(substr(body, p + 1)))
      if (section == "clusters") {
        if (k == "name") cname[item] = v
        else if (k == "certificate-authority-data") cdata[item] = v
        else if (k == "certificate-authority") cfile[item] = v
      } else {
        if (k == "name") xname[item] = v
        else if (k == "cluster") xcluster[item] = v
      }
    }
    END {
      if (cc == "") exit 2
      want = ""; n = 0
      for (i in xname) if (xname[i] == cc) { want = xcluster[i]; n++ }
      if (n != 1 || want == "") exit 3
      sel = ""; n = 0
      for (i in cname) if (cname[i] == want) { sel = i; n++ }
      if (n != 1) exit 4
      if (cdata[sel] != "") { printf "data %s\n", cdata[sel]; exit 0 }
      if (cfile[sel] != "") { printf "file %s\n", cfile[sel]; exit 0 }
      exit 5
    }
  ' "$1"
}

campaign_k8s_identity_refuse() {
  echo "campaign-distribute-k8s: REFUSED — peer cluster identity: $1" >&2
  echo "campaign-distribute-k8s: no Job was created. A campaign is never distributed onto an unverified cluster; pin the peer control plane in CAMPAIGN_PEER_CA_SHA256." >&2
  exit 16
}

campaign_k8s_verify_cluster_identity() {
  expected_raw="${CAMPAIGN_PEER_CA_SHA256:-}"
  [ -n "$expected_raw" ] || campaign_k8s_identity_refuse \
    "CAMPAIGN_PEER_CA_SHA256 is not set. There is no default peer cluster and no trust-on-first-use: an unpinned run would distribute onto whatever cluster happened to be reachable."

  expected="$(printf '%s' "$expected_raw" \
    | tr 'A-Z' 'a-z' | sed -e 's/^sha256://' -e 's/[: \t]//g' | tr -d '\n')"
  case "$expected" in
    *[!0-9a-f]*|'') campaign_k8s_identity_refuse "CAMPAIGN_PEER_CA_SHA256 is not a hex SHA256 digest" ;;
  esac
  [ "${#expected}" -eq 64 ] || campaign_k8s_identity_refuse \
    "CAMPAIGN_PEER_CA_SHA256 has ${#expected} hex digits, want 64 (a SHA256)"

  # The same file dks-profile.sh's peer profile resolves $KUBECTL against, so
  # the pin binds the transport that will actually run rather than a bystander
  # kubeconfig that merely happens to be lying around.
  kubeconfig="${DKS_PEER_KUBECONFIG:-${HOME:-}/.kube/outpost-control-plane/k3s.yaml}"
  [ -f "$kubeconfig" ] || campaign_k8s_identity_refuse \
    "peer kubeconfig not found at $kubeconfig (set DKS_PEER_KUBECONFIG)"

  set +e
  ca_ref="$(campaign_k8s_kubeconfig_ca_ref "$kubeconfig")"
  ca_rc=$?
  set -e
  case "$ca_rc" in
    0) ;;
    2) campaign_k8s_identity_refuse "the peer kubeconfig has no current-context, so there is no cluster to verify" ;;
    3) campaign_k8s_identity_refuse "the peer kubeconfig's current-context does not resolve to exactly one context" ;;
    4) campaign_k8s_identity_refuse "the current context's cluster does not resolve to exactly one cluster entry" ;;
    5) campaign_k8s_identity_refuse "the current context's cluster carries no certificate-authority — an unverifiable (e.g. insecure-skip-tls-verify) cluster cannot be pinned" ;;
    *) campaign_k8s_identity_refuse "the peer kubeconfig could not be read as a kubeconfig" ;;
  esac

  ca_kind="${ca_ref%% *}"
  ca_val="${ca_ref#* }"
  actual=''
  case "$ca_kind" in
    data)
      actual="$(printf '%s' "$ca_val" | campaign_k8s_b64_decode 2>/dev/null | campaign_k8s_sha256 2>/dev/null || true)"
      ;;
    file)
      [ -f "$ca_val" ] || campaign_k8s_identity_refuse \
        "the current context's cluster names a certificate-authority file that does not exist"
      actual="$(campaign_k8s_sha256 <"$ca_val" 2>/dev/null || true)"
      ;;
  esac
  case "$actual" in
    *[!0-9a-f]*|'') campaign_k8s_identity_refuse "the peer cluster's certificate-authority could not be fingerprinted" ;;
  esac
  [ "${#actual}" -eq 64 ] || campaign_k8s_identity_refuse \
    "the peer cluster's certificate-authority could not be fingerprinted"

  if [ "$actual" != "$expected" ]; then
    campaign_k8s_identity_refuse \
      "the kubeconfig this transport would use presents CA sha256 $actual, but the operator pinned $expected — this is not the peer control plane you pinned"
  fi
  echo "CAMPAIGN_K8S_PEER_IDENTITY_OK: peer cluster CA sha256 $actual matches the operator pin" >&2
}

# --- role -> node pin lookup -------------------------------------------------
#
# The persisted placement map (written ONCE by preflight) is authoritative when
# present, so an auto-resolved placement is never recomputed per chunk and a
# manual run reads the same resolved pins. Absent the map, fall back to the
# WORKER_NODES manual pins directly (a direct dispatch-chunk invocation with no
# preflight, e.g. in isolation).
campaign_k8s_node_for() {
  role="$1"
  if [ -n "${CAMPAIGN_K8S_PLACEMENT_FILE:-}" ] && [ -f "$CAMPAIGN_K8S_PLACEMENT_FILE" ]; then
    node="$(awk -F= -v r="$role" '$1 == r { print $2; exit }' "$CAMPAIGN_K8S_PLACEMENT_FILE")"
    if [ -n "$node" ]; then printf '%s\n' "$node"; return 0; fi
  fi
  for pair in ${WORKER_NODES:-}; do
    case "$pair" in
      "$role"=*) printf '%s\n' "${pair#*=}"; return 0 ;;
    esac
  done
  echo "campaign-distribute-k8s: no node pin for worker role '$role' (placement map / WORKER_NODES)" >&2
  return 1
}

# Manual-only lookup, used while BUILDING the placement map so it never reads a
# half-written map file.
campaign_k8s_worker_nodes_pin() {
  role="$1"
  for pair in ${WORKER_NODES:-}; do
    case "$pair" in
      "$role"=*) printf '%s\n' "${pair#*=}"; return 0 ;;
    esac
  done
  echo "campaign-distribute-k8s: no node pin for worker role '$role' in WORKER_NODES" >&2
  return 1
}

# --- auto placement: resolve each role to a distinct eligible node -----------
#
# Prints one `role=node` line per WORKERS role on stdout. Feeds prior choices
# through DKS_SELECT_EXCLUDE_NODES so a single point-in-time preflight cannot
# select the same node twice. Fails closed (non-zero) the moment the selector
# finds no eligible node for a role — insufficient / stale / unavailable
# capacity never degrades to an optimistic pin.
campaign_k8s_resolve_auto() {
  auto_os="${CAMPAIGN_SHARD_OS:-linux}"
  auto_backend="${CAMPAIGN_SHARD_BACKEND:-vk-podman}"
  auto_arch="${CAMPAIGN_SHARD_ARCH:-}"
  exclude=""
  for role in $WORKERS; do
    campaign_k8s_check_token "$role" "worker role"
    node="$(
      DKS_SELECT_OS="$auto_os" \
        DKS_SELECT_BACKEND="$auto_backend" \
        DKS_SELECT_ARCH="$auto_arch" \
        DKS_SELECT_CPU="$REQ_CPU" \
        DKS_SELECT_MEM="$REQ_MEM" \
        DKS_SELECT_EXCLUDE_NODES="$exclude" \
        "$root/dks-select-node.sh"
    )" || {
      echo "campaign-distribute-k8s: REFUSED — auto placement found no eligible node for worker role '$role' (backend=$auto_backend os=$auto_os arch=${auto_arch:-any} cpu=${REQ_CPU:-0} mem=${REQ_MEM:-0}; see dks-select-node reasons above). Insufficient/stale/unavailable capacity fails closed — no Job was created." >&2
      return 15
    }
    campaign_k8s_check_token "$node" "auto-selected node name"
    exclude="${exclude:+$exclude,}$node"
    echo "campaign-distribute-k8s: auto-placed role=$role node=$node (excluding: ${exclude})" >&2
    printf '%s=%s\n' "$role" "$node"
  done
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
  : "${CHUNK_IMAGE:?set CHUNK_IMAGE to the chunk-runner container image}"
  : "${CHUNK_POD_CMD:?set CHUNK_POD_CMD to the in-pod chunk runner}"

  auto=0
  if campaign_k8s_auto_mode; then auto=1; fi
  if [ "$auto" -eq 0 ]; then
    : "${WORKER_NODES:?set WORKER_NODES to space-separated role=node pins (manual override), or enable CAMPAIGN_AUTO_PLACEMENT=1 for resource-aware auto placement}"
  fi

  # Auto placement must persist its ONE resolution to a run-local file that the
  # separate dispatch-chunk process reads; without a shared path there is
  # nothing to share, so auto mode refuses rather than silently re-resolving.
  placement="${CAMPAIGN_K8S_PLACEMENT_FILE:-}"
  if [ "$auto" -eq 1 ] && [ -z "$placement" ]; then
    echo "campaign-distribute-k8s: REFUSED — auto placement needs CAMPAIGN_K8S_PLACEMENT_FILE, a run-local path where preflight persists the resolved role=node map for dispatch-chunk to read (campaign-distribute.sh sets it to \$OUT_DIR/placement.env)." >&2
    exit 20
  fi

  # If the per-shard requests are set, they must be plain tokens: they are
  # interpolated into the Job's resources.requests.
  [ -z "$REQ_CPU" ] || campaign_k8s_check_token "$REQ_CPU" "shard cpu request"
  [ -z "$REQ_MEM" ] || campaign_k8s_check_token "$REQ_MEM" "shard memory request"

  # Peer control-plane identity is verified before any node lookup, selector
  # run, or Job creation: reaching *a* cluster is not evidence of reaching the
  # right one.
  campaign_k8s_verify_cluster_identity

  # Resolve the role->node map ONCE and persist it (write-then-rename so a
  # reader never sees a partial map). dispatch-chunk reads $placement and never
  # re-resolves — the placement is decided here, exactly once.
  if [ -n "$placement" ]; then
    tmp_placement="$placement.tmp.$$"
    if [ "$auto" -eq 1 ]; then
      campaign_k8s_resolve_auto >"$tmp_placement" || { rc=$?; rm -f "$tmp_placement"; exit "$rc"; }
    else
      : >"$tmp_placement"
      for role in $WORKERS; do
        campaign_k8s_check_token "$role" "worker role"
        node="$(campaign_k8s_worker_nodes_pin "$role")" || { rm -f "$tmp_placement"; exit 10; }
        campaign_k8s_check_token "$node" "node name"
        printf '%s=%s\n' "$role" "$node" >>"$tmp_placement"
      done
    fi
    mv "$tmp_placement" "$placement"
  fi

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
    got="$(dks_kubectl get node "$node" -o jsonpath='{.metadata.name}' 2>/dev/null)" || {
      echo "campaign-distribute-k8s: REFUSED — worker role '$role' pins node '$node' which does not exist in the peer cluster" >&2
      exit 13
    }
    [ "$got" = "$node" ] || {
      echo "campaign-distribute-k8s: REFUSED — node lookup for '$node' returned '$got'" >&2
      exit 13
    }
    host="$(dks_kubectl get node "$node" -o jsonpath='{.metadata.labels.outpost\.dhnt\.io/host}' 2>/dev/null || true)"
    backend="$(dks_kubectl get node "$node" -o jsonpath='{.metadata.labels.outpost\.dhnt\.io/backend}' 2>/dev/null || true)"
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

  # resources.requests: the reservation Kubernetes enforces, sized to exactly
  # the per-shard requests the selector placed against, so the next selection
  # sees this shard as used capacity. Emitted only when a request is set (in
  # manual mode with no shard config it is absent — the manifest is unchanged).
  req_block=""
  if [ -n "$REQ_CPU" ] || [ -n "$REQ_MEM" ]; then
    req_block="          resources:
            requests:"
    [ -z "$REQ_CPU" ] || req_block="$req_block
              cpu: \"$REQ_CPU\""
    [ -z "$REQ_MEM" ] || req_block="$req_block
              memory: \"$REQ_MEM\""
  fi

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
${req_block}
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

  # Verify the peer control plane before creating this chunk's Job. This is
  # deliberately unconditional: it runs on the fake-kubectl seam too, so the
  # deterministic gate exercises the same code the real path does.
  campaign_k8s_verify_cluster_identity

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
  trap 'dks_kubectl delete job "$job" -n "$NS" --ignore-not-found --wait=false >/dev/null 2>&1 || true' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM

  campaign_k8s_manifest "$job" "$node" "$worker" "$chunk_id" "$cases" \
    | dks_kubectl apply -f - >/dev/null

  if ! dks_kubectl wait --for=condition=complete "--timeout=${CHUNK_TIMEOUT}s" "job/$job" -n "$NS" >/dev/null 2>&1; then
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id worker=$worker node=$node — Job $job did not complete within ${CHUNK_TIMEOUT}s (never scheduled, wedged, or failed); worker unreachable is not a pass" >&2
    exit 3
  fi

  job_uid="$(dks_kubectl get job "$job" -n "$NS" -o jsonpath='{.metadata.uid}')"
  [ -n "$job_uid" ] || {
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — Job $job has no uid; identity cannot be established" >&2
    exit 3
  }

  pods="$(dks_kubectl get pods -n "$NS" -l "job-name=$job" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')"
  pod_n="$(printf '%s' "$pods" | grep -c . || true)"
  if [ "$pod_n" -ne 1 ]; then
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — expected exactly 1 pod for Job $job, found $pod_n; ambiguous evidence is not evidence" >&2
    exit 3
  fi
  pod="$(printf '%s' "$pods" | head -n1)"

  owner_uid="$(dks_kubectl get pod "$pod" -n "$NS" -o jsonpath='{.metadata.ownerReferences[0].uid}')"
  if [ "$owner_uid" != "$job_uid" ]; then
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — pod $pod is not owned by Job $job (owner uid '$owner_uid' != job uid '$job_uid'); foreign evidence rejected" >&2
    exit 5
  fi

  observed_node="$(dks_kubectl get pod "$pod" -n "$NS" -o jsonpath='{.spec.nodeName}')"
  if [ "$observed_node" != "$node" ]; then
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — pinned node '$node' but pod $pod ran on '$observed_node'; the pin did not hold, placement evidence rejected" >&2
    exit 5
  fi

  log_file="$evidence_dir/chunk-$chunk_id.log"
  dks_kubectl logs "$pod" -n "$NS" >"$log_file" || {
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
  hdr_worker="$(printf '%s' "$header" | sed -n 's/.*"worker":"\([^"]*\)".*/\1/p')"
  hdr_suite="$(printf '%s' "$header" | sed -n 's/.*"suite":"\([^"]*\)".*/\1/p')"
  if [ "$hdr_job" != "$job" ] || [ "$hdr_chunk" != "$chunk_id" ] || [ "$hdr_node" != "$observed_node" ] || [ "$hdr_worker" != "$worker" ] || [ "$hdr_suite" != "$SUITE" ]; then
    echo "campaign-distribute-k8s: FAIL chunk=$chunk_id — evidence header (job=$hdr_job chunk=$hdr_chunk node=$hdr_node worker=$hdr_worker suite=$hdr_suite) does not match Job $job chunk $chunk_id node $observed_node worker=$worker suite=$SUITE" >&2
    exit 5
  fi

  sed '1d' "$log_file" >"$out_file"

  # An evidence sidecar is often archived away from the run line that
  # produced it, so it must self-identify whether a fake kubectl produced it
  # rather than relying on context that will not travel with the file.
  if [ -n "${CAMPAIGN_K8S_FAKE_KUBECTL:-}" ]; then
    sidecar_transport=k8s-fake-kubectl
    sidecar_evidence=logic-check-only
  else
    sidecar_transport=k8s-peer
    sidecar_evidence=development-only
  fi

  printf '%s\n' "$observed_node" >"$evidence_dir/chunk-$chunk_id.node"
  printf '{"schema":"bashy.campaign.evidence/v1","suite":"%s","chunk":%s,"worker":"%s","job":"%s","job_uid":"%s","pod":"%s","node":"%s","log":"%s","transport":"%s","evidence":"%s"}\n' \
    "$SUITE" "$chunk_id" "$worker" "$job" "$job_uid" "$pod" "$observed_node" "$log_file" \
    "$sidecar_transport" "$sidecar_evidence" \
    >"$evidence_dir/chunk-$chunk_id.evidence.json"

  dks_kubectl delete job "$job" -n "$NS" --ignore-not-found --wait=false >/dev/null
  trap - EXIT
}

# --- cleanup: delete every Job the ledger records ------------------------------
campaign_k8s_cleanup() {
  ledger="$1"
  [ -f "$ledger" ] || return 0
  while IFS= read -r job; do
    [ -n "$job" ] || continue
    dks_kubectl delete job "$job" -n "$NS" --ignore-not-found --wait=false >/dev/null 2>&1 || true
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
