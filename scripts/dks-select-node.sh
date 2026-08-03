#!/usr/bin/env bash
# dks-select-node.sh - resource-aware placement for DKS test/build shards.
#
# Prints EXACTLY ONE line on stdout: the name of the node a shard should run on.
# Everything else (candidate scoring, exclusion reasons, metrics warnings) goes
# to stderr. Nothing it prints is derived from a kubectl argv, a kubeconfig, or
# any JSON payload beyond node/pod NAMES and integer resource quantities - see
# SECRET SAFETY below.
#
#   node="$(DKS_SELECT_OS=darwin DKS_SELECT_CPU=2 DKS_SELECT_MEM=4Gi \
#            scripts/dks-select-node.sh)"
#
# WHY: Dragon is the fleet's coordination seat. Left to the default scheduler,
# every native shard lands wherever the API server feels like putting it, which
# in practice has been Dragon - the one node that must stay responsive to run
# the conductor. This selector reserves Dragon (and any control-plane node)
# behind a score penalty and prefers idle capable workers (novicortex,
# novidesign by default), while still failing closed rather than placing a shard
# on a node that cannot actually hold it.
#
# FAIL-CLOSED, ALWAYS. Every unknown is an error, never an optimistic default:
#   - no eligible node                       -> exit 3 (with per-node reasons)
#   - a node's usage is unknown              -> the node's RESERVED REQUESTS are
#                                               used and the fallback is WARNED
#                                               (never treated as zero usage)
#   - metrics missing + REQUIRE_METRICS=1    -> exit 6
#   - the cluster query fails                -> exit 5
#   - DKS_PROFILE=peer with no kubeconfig    -> exit 9 from dks-profile.sh; this
#                                               script NEVER falls back to the
#                                               cloudbox profile
#   - an unparseable quantity                -> exit 2 (not silently zero)
#
# ---------------------------------------------------------------------------
# Inputs (all environment; no flags, so it is safe to call from a heredoc).
#
# Requirement / filtering:
#   DKS_SELECT_OS          REQUIRED. kubernetes.io/os of the target node.
#   DKS_SELECT_BACKEND     outpost.dhnt.io/backend label (default vk-native).
#   DKS_SELECT_ARCH        kubernetes.io/arch, when the shard is arch-specific.
#   DKS_SELECT_HOST        exact outpost.dhnt.io/host, when the shard is pinned
#                          to one registered host. Still resource-checked.
#   DKS_SELECT_CPU         CPU the shard needs, as a k8s quantity (e.g. 2, 500m).
#   DKS_SELECT_MEM         memory the shard needs (e.g. 4Gi, 512Mi).
#   DKS_SELECT_EXCLUDE_NODES
#                          comma-separated node names to skip. This is how a
#                          fan-out loop spreads shards it has ALREADY placed but
#                          whose Pods do not exist yet - see CONCURRENCY below.
#
# Preference / reserve:
#   DKS_SELECT_PREFERRED_HOSTS   default "novicortex,novidesign".
#   DKS_SELECT_RESERVED_HOSTS    default "dragon".
#   DKS_SELECT_PREFERRED_BONUS   default 25 score points.
#   DKS_SELECT_RESERVE_PENALTY   default 60 score points (also applied to any
#                                node labelled node-role.kubernetes.io/
#                                control-plane or .../master).
#   DKS_SELECT_SPREAD_SELECTOR   label "key=value" whose active Pods count as
#                                sibling shards (default
#                                dhnt.io/lane=native-platform). Empty disables.
#   DKS_SELECT_SPREAD_PENALTY    default 35 points per sibling shard already on
#                                the node. This is what keeps two shards off one
#                                node while another eligible node is idle.
#
# Metrics:
#   DKS_SELECT_REQUIRE_METRICS   1 = a node with no metrics.k8s.io reading is an
#                                ERROR (exit 6) rather than a warned fallback.
#
# Hermetic fixtures (offline testing; when NODES_JSON is set NO kubectl runs):
#   DKS_SELECT_NODES_JSON        path to a `kubectl get nodes -o json` payload.
#   DKS_SELECT_PODS_JSON         path to a `kubectl get pods -A -o json` payload.
#                                Unset, with fixtures, means "no pods".
#   DKS_SELECT_METRICS_JSON      path to a metrics.k8s.io NodeMetricsList.
#                                Unset means METRICS UNAVAILABLE - which is a
#                                warned requests-only fallback, not zero usage.
#
# Output:
#   DKS_SELECT_DIAG_FILE   optional path. Receives non-secret `key=value` lines
#                          describing the decision (node, host, the label to pin
#                          placement with, headroom, usage source). This is how
#                          dks-native-job.sh turns a node name into a nodeSelector.
#
# ---------------------------------------------------------------------------
# SCORING. For each surviving candidate, per dimension:
#
#   used  = max(live usage from metrics.k8s.io, sum of active Pod requests)
#   free  = allocatable - used
#
# The max() is the conservative choice and it is load-bearing in BOTH
# directions: requests alone miss a process that is burning four cores under a
# 500m request, and live usage alone misses a Pod that has been admitted but has
# not started consuming yet. Taking either one on its own overcommits a node.
#
# A candidate must satisfy free >= required on BOTH dimensions or it is dropped
# as insufficient. The score is then the SMALLER of the two post-placement
# headroom percentages - the binding dimension, because a node with 90% CPU free
# and 2% memory free is not a good host for anything.
#
#   score = min(cpu_headroom_pct, mem_headroom_pct)
#           + preferred_bonus - reserve_penalty - spread_penalty*siblings
#
# Ties break deterministically and totally: score desc, absolute CPU headroom
# desc, absolute memory headroom desc, then node name ascending. The last key is
# a total order over distinct nodes, so the selection is a pure function of the
# input JSON - the same fixtures always choose the same node.
#
# ---------------------------------------------------------------------------
# CONCURRENCY - the honest limitation.
#
# This is a POINT-IN-TIME selector, not a scheduler and not a lock. It reads the
# cluster as it is at the instant it runs. Two selections that run CONCURRENTLY,
# before either shard's Pod exists, see the same snapshot and can pick the same
# node. There is no reservation, no lease, and no admission control here.
#
# What actually holds in practice, and what does not:
#   - SEQUENTIAL fan-out is correct. dks-native-job.sh stamps real
#     resources.requests onto each Pod, so once a shard's Pod is created its
#     request is visible to the next selection as reserved capacity, even before
#     the process starts burning anything.
#   - CONCURRENT fan-out needs DKS_SELECT_EXCLUDE_NODES. A launcher that emits
#     several manifests before applying any of them must feed back the nodes it
#     already chose. Nothing in this script can do that for it.
#   - The spread penalty helps but does not fix it: it counts Pods that EXIST.
#   - Kubernetes remains the final authority. This script only narrows
#     placement; the scheduler can still reject the choice, and a Pod that will
#     not fit stays Pending rather than being silently placed elsewhere.
#
# SECRET SAFETY: this session may run under unredacted PTY capture. A kubectl
# argv can carry `--token=`, a password, or a credentialed URL, so this script
# never echoes the resolved argv, `$KUBECTL`, or `$DKS_KUBECTL_ARGV`, on any
# path - it delegates every invocation to dks-profile.sh's `dks_kubectl`, which
# execs the stored argv without printing it. Diagnostics name only node/host
# names and integer quantities; raw JSON is never echoed, not even on a parse
# failure (a failure names the SOURCE, not the payload, since a cluster dump can
# contain Secret-shaped environment values).
#
# Tests: scripts/test-dks-select-node.sh (fixture JSON only - no cluster, no
# kubectl execution, no network). Design: docs/plan-dks-build-test-deploy.md.
#
# `$0`-based self-location, never `${BASH_SOURCE[0]:-$0}`: Bashy does not
# populate BASH_SOURCE[0] for a top-level script and that idiom nil-derefs its
# expander under `set -u` (see dks-profile.sh).
set -euo pipefail

_here=$(cd "$(dirname "$0")" && pwd)
. "$_here/dks-profile.sh"

# Deterministic sorting/collation regardless of the operator's locale.
LC_ALL=C
export LC_ALL

# Field separator for every intermediate record, computed once.
#
# NOT a tab. A tab is an IFS *whitespace* character, and POSIX word splitting
# collapses a RUN of IFS whitespace into a single delimiter — so `a<tab><tab>b`
# reads as two fields, not three, and every EMPTY field silently disappears,
# shifting each later column left. That is not hypothetical: a node with no
# `outpost.dhnt.io/host` and no `kubernetes.io/hostname` label emits two adjacent
# empty fields, which shifted its allocatable CPU into the Ready-condition slot
# and made a perfectly healthy node get skipped as `not-ready condition=32`.
# The same collapse silently mis-assigned pin_label/pin_value for any node
# pinned by hostname rather than registered host.
#
# ASCII US (0x1f) is not IFS whitespace, so empty fields survive. Kubernetes
# node names are DNS subdomains and label values are alphanumeric+`-_.`, so
# neither can contain it.
US=$(printf '\037')

BACKEND="${DKS_SELECT_BACKEND:-vk-native}"
WANT_OS="${DKS_SELECT_OS:-}"
WANT_ARCH="${DKS_SELECT_ARCH:-}"
WANT_HOST="${DKS_SELECT_HOST:-}"
NEED_CPU_Q="${DKS_SELECT_CPU:-0}"
NEED_MEM_Q="${DKS_SELECT_MEM:-0}"
# Single-dash defaults: an explicitly-EMPTY list means "no preferred hosts" /
# "reserve nothing", which is a legitimate operator choice. Only a genuinely
# unset variable falls back to the fleet defaults.
PREFERRED_HOSTS="${DKS_SELECT_PREFERRED_HOSTS-novicortex,novidesign}"
RESERVED_HOSTS="${DKS_SELECT_RESERVED_HOSTS-dragon}"
PREFERRED_BONUS="${DKS_SELECT_PREFERRED_BONUS:-25}"
RESERVE_PENALTY="${DKS_SELECT_RESERVE_PENALTY:-60}"
SPREAD_SELECTOR="${DKS_SELECT_SPREAD_SELECTOR-dhnt.io/lane=native-platform}"
SPREAD_PENALTY="${DKS_SELECT_SPREAD_PENALTY:-35}"
REQUIRE_METRICS="${DKS_SELECT_REQUIRE_METRICS:-0}"
EXCLUDE_NODES="${DKS_SELECT_EXCLUDE_NODES:-}"
DIAG_FILE="${DKS_SELECT_DIAG_FILE:-}"

# The one toleration DKS Jobs carry (see dks-native-job.sh). A node whose taints
# are not all covered by this is not schedulable for us.
TOLERATED_TAINT="${DKS_SELECT_TOLERATED_TAINT:-virtual-kubelet.io/provider=outpost:NoSchedule}"

say() { printf 'dks-select-node: %s\n' "$*" >&2; }
die() { # rc, message
  local rc="$1"
  shift
  say "$*"
  exit "$rc"
}

[ -n "$WANT_OS" ] || die 2 "DKS_SELECT_OS is required (linux, darwin, or windows)"
case "$WANT_OS" in
  linux | darwin | windows) ;;
  *) die 2 "DKS_SELECT_OS must be linux, darwin, or windows (got '$WANT_OS')" ;;
esac
for n in "$PREFERRED_BONUS" "$RESERVE_PENALTY" "$SPREAD_PENALTY"; do
  case "$n" in
    '' | *[!0-9]*) die 2 "score tuning values must be non-negative integers (got '$n')" ;;
  esac
done

command -v jq >/dev/null 2>&1 || die 4 \
  "jq not found. Bashy ships jq in its pure-Go userland; run this under bashy or install jq."

# ---------------------------------------------------------------------------
# Quantity parsing. Kubernetes quantities are not plain integers: allocatable
# arrives as "8" or "7900m" CPU and "16Gi" memory, while metrics.k8s.io reports
# CPU in NANOCORES ("1234567890n") and memory in "Ki". Every one of those has to
# land in the same integer unit before any comparison is meaningful.

# _scale VALUE MULTIPLIER - floor(VALUE * MULTIPLIER) for a possibly-decimal
# VALUE, in integer arithmetic (no bc, no floating point, no locale decimal
# separator surprises).
_scale() {
  local v="$1" mult="$2" int frac digits pow=1 i=0
  case "$v" in
    *.*)
      int="${v%%.*}"
      frac="${v#*.}"
      ;;
    *)
      int="$v"
      frac=""
      ;;
  esac
  [ -n "$int" ] || int=0
  frac="${frac:0:9}"
  digits=${#frac}
  while [ "$i" -lt "$digits" ]; do
    pow=$((pow * 10))
    i=$((i + 1))
  done
  printf '%s' $((int * mult + (10#0${frac:-0} * mult) / pow))
}

# _numeric VALUE - reject anything that is not a bare decimal number. An
# unparseable quantity must be an ERROR, not a zero: a node whose allocatable
# silently read as 0 would be excluded as "too small" and a node whose usage
# silently read as 0 would look infinitely idle. Both are wrong answers that
# look like working answers.
_numeric() {
  case "$1" in
    '' | *[!0-9.]*) return 1 ;;
    *.*.*) return 1 ;;
  esac
  return 0
}

# parse_cpu_milli QUANTITY [WHAT] -> millicores
parse_cpu_milli() {
  local q="$1" what="${2:-cpu quantity}" v out
  case "$q" in
    '') printf '0'; return 0 ;;
    *n) # nanocores (metrics.k8s.io) - round UP, conservative for usage
      v="${q%n}"
      _numeric "$v" || die 2 "unparseable $what '$q'"
      out=$(_scale "$v" 1)
      printf '%s' $(((out + 999999) / 1000000))
      ;;
    *u) # microcores - round UP
      v="${q%u}"
      _numeric "$v" || die 2 "unparseable $what '$q'"
      out=$(_scale "$v" 1)
      printf '%s' $(((out + 999) / 1000))
      ;;
    *m)
      v="${q%m}"
      _numeric "$v" || die 2 "unparseable $what '$q'"
      _scale "$v" 1
      ;;
    *)
      _numeric "$q" || die 2 "unparseable $what '$q'"
      _scale "$q" 1000
      ;;
  esac
}

# parse_mem_bytes QUANTITY [WHAT] -> bytes
parse_mem_bytes() {
  local q="$1" what="${2:-memory quantity}" v base exp mult
  case "$q" in
    '') printf '0'; return 0 ;;
    *Ki) v="${q%Ki}"; mult=1024 ;;
    *Mi) v="${q%Mi}"; mult=1048576 ;;
    *Gi) v="${q%Gi}"; mult=1073741824 ;;
    *Ti) v="${q%Ti}"; mult=1099511627776 ;;
    *Pi) v="${q%Pi}"; mult=1125899906842624 ;;
    *K | *k) v="${q%?}"; mult=1000 ;;
    *M) v="${q%M}"; mult=1000000 ;;
    *G) v="${q%G}"; mult=1000000000 ;;
    *T) v="${q%T}"; mult=1000000000000 ;;
    *P) v="${q%P}"; mult=1000000000000000 ;;
    *e*)
      # k8s canonical exponent form, e.g. 128974848e0.
      base="${q%%e*}"
      exp="${q#*e}"
      _numeric "$base" || die 2 "unparseable $what '$q'"
      case "$exp" in '' | *[!0-9]*) die 2 "unparseable $what '$q'" ;; esac
      mult=1
      while [ "$exp" -gt 0 ]; do
        mult=$((mult * 10))
        exp=$((exp - 1))
      done
      _scale "$base" "$mult"
      return 0
      ;;
    *) v="$q"; mult=1 ;;
  esac
  _numeric "$v" || die 2 "unparseable $what '$q'"
  _scale "$v" "$mult"
}

NEED_CPU=$(parse_cpu_milli "$NEED_CPU_Q" "DKS_SELECT_CPU")
NEED_MEM=$(parse_mem_bytes "$NEED_MEM_Q" "DKS_SELECT_MEM")

# ---------------------------------------------------------------------------
# Cluster state. Auto-placement must never follow the mutable ambient context
# back to cloudbox. An unset profile means peer for this selector only;
# explicit argv overrides remain available. A fixture-only run needs no
# kubectl/profile resolution unless it is explicitly testing one.
fixture_mode=0
[ -n "${DKS_SELECT_NODES_JSON:-}" ] && fixture_mode=1
if [ -z "${KUBECTL:-}" ] && [ -z "${DKS_KUBECTL_ARGV:-}" ]; then
  if [ "$fixture_mode" -eq 0 ] || [ "${DKS_PROFILE+x}" = x ]; then
    profile="${DKS_PROFILE-peer}"
    [ "$profile" = peer ] || die 9 "auto node selection requires DKS_PROFILE=peer; refusing profile '${profile:-<empty>}'"
    DKS_PROFILE=peer
    export DKS_PROFILE
    dks_kubectl_init "bashy kubectl" || exit $?
  fi
else
  dks_kubectl_init "bashy kubectl" || exit $?
fi

tmp=$(mktemp -d)
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

nodes_json="$tmp/nodes.json"
pods_json="$tmp/pods.json"
metrics_json="$tmp/metrics.json"
metrics_available=0

if [ -n "${DKS_SELECT_NODES_JSON:-}" ]; then
  [ -r "${DKS_SELECT_NODES_JSON}" ] ||
    die 5 "DKS_SELECT_NODES_JSON is not readable: ${DKS_SELECT_NODES_JSON}"
  cat "${DKS_SELECT_NODES_JSON}" >"$nodes_json"
  if [ -n "${DKS_SELECT_PODS_JSON:-}" ]; then
    [ -r "${DKS_SELECT_PODS_JSON}" ] ||
      die 5 "DKS_SELECT_PODS_JSON is not readable: ${DKS_SELECT_PODS_JSON}"
    cat "${DKS_SELECT_PODS_JSON}" >"$pods_json"
  else
    printf '{"items":[]}' >"$pods_json"
  fi
  if [ -n "${DKS_SELECT_METRICS_JSON:-}" ]; then
    [ -r "${DKS_SELECT_METRICS_JSON}" ] ||
      die 5 "DKS_SELECT_METRICS_JSON is not readable: ${DKS_SELECT_METRICS_JSON}"
    cat "${DKS_SELECT_METRICS_JSON}" >"$metrics_json"
    metrics_available=1
  fi
  say "reading fixture cluster state (no kubectl invoked)"
else
  dks_kubectl get nodes -o json >"$nodes_json" 2>/dev/null ||
    die 5 "failed to query nodes from the cluster"
  dks_kubectl get pods --all-namespaces -o json >"$pods_json" 2>/dev/null ||
    die 5 "failed to query pods from the cluster"
  # metrics.k8s.io is an OPTIONAL aggregated API. Its absence is a documented
  # fallback, never a silent zero.
  if dks_kubectl get --raw /apis/metrics.k8s.io/v1beta1/nodes >"$metrics_json" 2>/dev/null; then
    metrics_available=1
  fi
fi

# Validate up front so a truncated payload is a clean error rather than an empty
# candidate list that reads as "no capacity". The payload itself is never
# echoed - a cluster dump can carry Secret-shaped values.
jq -e 'type == "object" and (.items | type) == "array"' "$nodes_json" >/dev/null 2>&1 ||
  die 5 "node inventory is not a valid Kubernetes list payload"
jq -e 'type == "object" and (.items | type) == "array"' "$pods_json" >/dev/null 2>&1 ||
  die 5 "pod inventory is not a valid Kubernetes list payload"
if [ "$metrics_available" -eq 1 ]; then
  jq -e 'type == "object" and (.items | type) == "array"' "$metrics_json" >/dev/null 2>&1 ||
    die 5 "node metrics payload is not a valid NodeMetricsList"
fi

if [ "$metrics_available" -eq 0 ]; then
  if [ "$REQUIRE_METRICS" = 1 ]; then
    die 6 "metrics.k8s.io is unavailable and DKS_SELECT_REQUIRE_METRICS=1"
  fi
  say "WARNING: metrics.k8s.io is unavailable; scoring falls back to RESERVED POD REQUESTS only. Live usage above a node's requests is INVISIBLE to this run - it is not being treated as zero."
fi

# ---------------------------------------------------------------------------
# Helpers over the loaded state. Each takes a node name through the environment
# (`env.NODE`) rather than jq --arg: Bashy's pure-Go jq implements env.VAR but
# not --arg/--argjson, and this script must run under bashy.

# Active pod requests on a node, one "pod-id kind cpu mem" line each.
pod_requests_lines() {
  NODE="$1" jq -r '
    .items[]
    | select(.spec.nodeName == env.NODE)
    | select(.status.phase == "Running" or .status.phase == "Pending")
    | . as $p
    | (($p.metadata.uid // (($p.metadata.namespace // "") + "/" + $p.metadata.name))) as $id
    | ( ($p.spec.containers // [])[]
        | [$id, "c", (.resources.requests.cpu // "0"), (.resources.requests.memory // "0")] ),
      ( ($p.spec.initContainers // [])[]
        | [$id, "i", (.resources.requests.cpu // "0"), (.resources.requests.memory // "0")] ),
      [$id, "o", ($p.spec.overhead.cpu // "0"), ($p.spec.overhead.memory // "0")]
    | map(tostring) | join("\u001f")
  ' "$pods_json"
}

# count of active sibling shards on a node (label key=value match)
spread_count() {
  local node="$1" key val
  [ -n "$SPREAD_SELECTOR" ] || { printf '0'; return 0; }
  case "$SPREAD_SELECTOR" in
    *=*) key="${SPREAD_SELECTOR%%=*}"; val="${SPREAD_SELECTOR#*=}" ;;
    *) printf '0'; return 0 ;;
  esac
  NODE="$node" SKEY="$key" SVAL="$val" jq -r '
    [ .items[]
      | select(.spec.nodeName == env.NODE)
      | select(.status.phase == "Running" or .status.phase == "Pending")
      | select((.metadata.labels // {})[env.SKEY] == env.SVAL)
    ] | length
  ' "$pods_json"
}

# live usage for a node: "cpuQty memQty source", empty when neither the node
# nor its physical host has a reading. Virtual nodes have no kubelet and thus
# no NodeMetrics object of their own; for them, use the physical k3s/agent Node
# carrying the same outpost host label. That is the utilization of the machine
# where vk-native/vk-podman will actually execute.
node_usage() {
  [ "$metrics_available" -eq 1 ] || return 0
  local name="$1" host="${2:-}" usage host_node
  usage=$(NODE="$name" jq -r '
    .items[]
    | select(.metadata.name == env.NODE)
    | [(.usage.cpu // ""), (.usage.memory // ""), "metrics"] | map(tostring) | join("\u001f")
  ' "$metrics_json" | head -n 1)
  if [ -n "$usage" ]; then
    printf '%s\n' "$usage"
    return 0
  fi
  [ -n "$host" ] || return 0
  host_node=$(HOST="$host" jq -r '
    .items[]
    | select((.metadata.labels["outpost.dhnt.io/host"] // "") == env.HOST)
    | select((.metadata.labels["outpost.dhnt.io/runtime"] // "") != "virtual")
    | .metadata.name
  ' "$nodes_json" | head -n 1)
  [ -n "$host_node" ] || return 0
  NODE="$host_node" jq -r '
    .items[]
    | select(.metadata.name == env.NODE)
    | [(.usage.cpu // ""), (.usage.memory // ""), "host-metrics"] | map(tostring) | join("\u001f")
  ' "$metrics_json" | head -n 1
}

# list_contains LIST NAME HOST - comma-separated LIST matched against a node
# name or its registered host label, case-insensitively. A bare entry also
# matches a `<entry>-suffix` / `<entry>.suffix` node name, so "dragon" covers a
# node registered as `dragon-vk-native` without the operator having to know the
# exact registration spelling.
list_contains() {
  local list="$1" name="$2" host="$3" entry rest
  [ -n "$list" ] || return 1
  name=$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')
  host=$(printf '%s' "$host" | tr '[:upper:]' '[:lower:]')
  rest="$list,"
  while [ -n "$rest" ]; do
    entry="${rest%%,*}"
    rest="${rest#*,}"
    [ -n "$entry" ] || continue
    entry=$(printf '%s' "$entry" | tr '[:upper:]' '[:lower:]')
    if [ "$name" = "$entry" ]; then return 0; fi
    if [ -n "$host" ] && [ "$host" = "$entry" ]; then return 0; fi
    case "$name" in "$entry"-* | "$entry".*) return 0 ;; esac
    case "$host" in "$entry"-* | "$entry".*) return 0 ;; esac
  done
  return 1
}

# ---------------------------------------------------------------------------
# Candidate evaluation.

candidates="$tmp/candidates"
: >"$candidates"
considered=0
metrics_missing_nodes=""

# One TSV line per node: name, backend, os, arch, host, hostname, unschedulable,
# ready, control-plane, allocatable cpu, allocatable memory.
node_lines=$(jq -r '
  .items[]
  | .metadata.labels as $l
  | [ .metadata.name,
      ($l["outpost.dhnt.io/backend"] // ""),
      ($l["kubernetes.io/os"] // ""),
      ($l["kubernetes.io/arch"] // ""),
      ($l["outpost.dhnt.io/host"] // ""),
      ($l["kubernetes.io/hostname"] // ""),
      ((.spec.unschedulable // false) | tostring),
      ([ (.status.conditions // [])[] | select(.type == "Ready") | .status ] | join("")),
      (if ($l | has("node-role.kubernetes.io/control-plane"))
          or ($l | has("node-role.kubernetes.io/master")) then "1" else "0" end),
      (.status.allocatable.cpu // "0"),
      (.status.allocatable.memory // "0")
    ] | map(tostring) | join("\u001f")
' "$nodes_json")

[ -n "$node_lines" ] || die 3 "the node inventory contains no nodes at all"

while IFS="$US" read -r name backend nos narch nhost nhostname unsched ready cplane alloc_cpu_q alloc_mem_q; do
  [ -n "$name" ] || continue
  considered=$((considered + 1))

  if list_contains "$EXCLUDE_NODES" "$name" "$nhost"; then
    say "skip node=$name reason=explicitly-excluded"
    continue
  fi
  if [ "$backend" != "$BACKEND" ]; then
    say "skip node=$name reason=backend-mismatch want=$BACKEND got=${backend:-<none>}"
    continue
  fi
  if [ "$nos" != "$WANT_OS" ]; then
    say "skip node=$name reason=os-mismatch want=$WANT_OS got=${nos:-<none>}"
    continue
  fi
  if [ -n "$WANT_ARCH" ] && [ "$narch" != "$WANT_ARCH" ]; then
    say "skip node=$name reason=arch-mismatch want=$WANT_ARCH got=${narch:-<none>}"
    continue
  fi
  if [ -n "$WANT_HOST" ] && [ "$nhost" != "$WANT_HOST" ]; then
    say "skip node=$name reason=host-mismatch want=$WANT_HOST got=${nhost:-<none>}"
    continue
  fi
  if [ "$ready" != "True" ]; then
    say "skip node=$name reason=not-ready condition=${ready:-<absent>}"
    continue
  fi
  if [ "$unsched" = true ]; then
    say "skip node=$name reason=cordoned"
    continue
  fi

  # Taints. A node carrying any taint this Job's single toleration does not
  # cover cannot host the shard, so scoring it would produce a node name the
  # scheduler then refuses - a selection that looks successful and is not.
  blocking=""
  taint_lines=$(NODE="$name" jq -r '
    .items[]
    | select(.metadata.name == env.NODE)
    | ((.spec.taints // [])[] | "\(.key)=\(.value // "" ):\(.effect)")
  ' "$nodes_json")
  if [ -n "$taint_lines" ]; then
    while IFS= read -r taint; do
      [ -n "$taint" ] || continue
      case "$taint" in
        *:PreferNoSchedule) continue ;;
      esac
      if [ "$taint" = "$TOLERATED_TAINT" ]; then continue; fi
      blocking="$taint"
      break
    done <<EOF
$taint_lines
EOF
  fi
  if [ -n "$blocking" ]; then
    say "skip node=$name reason=untolerated-taint taint=$blocking"
    continue
  fi

  alloc_cpu=$(parse_cpu_milli "$alloc_cpu_q" "allocatable cpu on node $name")
  alloc_mem=$(parse_mem_bytes "$alloc_mem_q" "allocatable memory on node $name")
  if [ "$alloc_cpu" -le 0 ] || [ "$alloc_mem" -le 0 ]; then
    say "skip node=$name reason=no-allocatable-capacity cpu=$alloc_cpu_q memory=$alloc_mem_q"
    continue
  fi

  # Reserved: Kubernetes computes max(sum(containers), max(initContainers))
  # plus overhead PER POD, then sums Pods. A global init max undercounts
  # multiple init-heavy Pods.
  req_cpu=0
  req_mem=0
  pod_id=""
  pod_cpu=0; pod_mem=0; pod_init_cpu=0; pod_init_mem=0
  pod_overhead_cpu=0; pod_overhead_mem=0
  flush_pod_request() {
    [ -n "$pod_id" ] || return 0
    local effective_cpu="$pod_cpu" effective_mem="$pod_mem"
    [ "$pod_init_cpu" -gt "$effective_cpu" ] && effective_cpu="$pod_init_cpu"
    [ "$pod_init_mem" -gt "$effective_mem" ] && effective_mem="$pod_init_mem"
    req_cpu=$((req_cpu + effective_cpu + pod_overhead_cpu))
    req_mem=$((req_mem + effective_mem + pod_overhead_mem))
  }
  req_lines=$(pod_requests_lines "$name")
  if [ -n "$req_lines" ]; then
    while IFS="$US" read -r pid kind cq mq; do
      [ -n "$pid" ] || continue
      if [ -n "$pod_id" ] && [ "$pid" != "$pod_id" ]; then
        flush_pod_request
        pod_cpu=0; pod_mem=0; pod_init_cpu=0; pod_init_mem=0
        pod_overhead_cpu=0; pod_overhead_mem=0
      fi
      pod_id="$pid"
      c=$(parse_cpu_milli "$cq" "pod cpu request on node $name")
      m=$(parse_mem_bytes "$mq" "pod memory request on node $name")
      case "$kind" in
        c) pod_cpu=$((pod_cpu + c)); pod_mem=$((pod_mem + m)) ;;
        i)
          [ "$c" -gt "$pod_init_cpu" ] && pod_init_cpu=$c
          [ "$m" -gt "$pod_init_mem" ] && pod_init_mem=$m
          ;;
        o) pod_overhead_cpu=$c; pod_overhead_mem=$m ;;
      esac
    done <<EOF
$req_lines
EOF
    flush_pod_request
  fi

  # Live usage, when the metrics API has a reading for THIS node.
  usage_src=requests
  use_cpu=0
  use_mem=0
  usage=$(node_usage "$name" "$nhost")
  if [ -n "$usage" ]; then
    ucq="${usage%%"$US"*}"
    usage_rest="${usage#*"$US"}"
    umq="${usage_rest%%"$US"*}"
    reported_usage_src="${usage_rest#*"$US"}"
    use_cpu=$(parse_cpu_milli "$ucq" "metrics cpu usage on node $name")
    use_mem=$(parse_mem_bytes "$umq" "metrics memory usage on node $name")
    usage_src="$reported_usage_src"
  elif [ "$metrics_available" -eq 1 ]; then
    if [ "$REQUIRE_METRICS" = 1 ]; then
      die 6 "node $name has no metrics.k8s.io reading and DKS_SELECT_REQUIRE_METRICS=1"
    fi
    metrics_missing_nodes="$metrics_missing_nodes $name"
    say "WARNING: node=$name has no metrics.k8s.io reading; using its reserved requests. Its live usage is UNKNOWN, not zero."
  fi

  # The conservative max(): whichever of "what is reserved" and "what is
  # actually being burned" is larger is the number that governs.
  used_cpu=$req_cpu
  if [ "$use_cpu" -gt "$used_cpu" ]; then used_cpu=$use_cpu; fi
  used_mem=$req_mem
  if [ "$use_mem" -gt "$used_mem" ]; then used_mem=$use_mem; fi

  free_cpu=$((alloc_cpu - used_cpu))
  free_mem=$((alloc_mem - used_mem))
  if [ "$free_cpu" -lt 0 ]; then free_cpu=0; fi
  if [ "$free_mem" -lt 0 ]; then free_mem=0; fi

  if [ "$free_cpu" -lt "$NEED_CPU" ] || [ "$free_mem" -lt "$NEED_MEM" ]; then
    say "skip node=$name reason=insufficient-capacity needCpuMilli=$NEED_CPU freeCpuMilli=$free_cpu needMemMiB=$((NEED_MEM / 1048576)) freeMemMiB=$((free_mem / 1048576)) usage=$usage_src"
    continue
  fi

  cpu_pct=$(((free_cpu - NEED_CPU) * 100 / alloc_cpu))
  mem_pct=$(((free_mem - NEED_MEM) * 100 / alloc_mem))
  score=$cpu_pct
  if [ "$mem_pct" -lt "$score" ]; then score=$mem_pct; fi

  role=worker
  if list_contains "$PREFERRED_HOSTS" "$name" "$nhost"; then
    score=$((score + PREFERRED_BONUS))
    role=preferred
  fi
  if list_contains "$RESERVED_HOSTS" "$name" "$nhost" || [ "$cplane" = 1 ]; then
    score=$((score - RESERVE_PENALTY))
    role=reserved
  fi
  siblings=$(spread_count "$name")
  case "$siblings" in '' | *[!0-9]*) siblings=0 ;; esac
  if [ "$siblings" -gt 0 ]; then
    score=$((score - SPREAD_PENALTY * siblings))
  fi

  # Pin placement by the registered-host label when the node has one (that is
  # the vocabulary the rest of the DKS scripts already speak), else by the
  # well-known hostname label. A node with neither cannot be pinned, and an
  # unpinnable selection would emit a node name the Job could not honor.
  pin_label=""
  pin_value=""
  if [ -n "$nhost" ]; then
    pin_label="outpost.dhnt.io/host"
    pin_value="$nhost"
  elif [ -n "$nhostname" ]; then
    pin_label="kubernetes.io/hostname"
    pin_value="$nhostname"
  else
    say "skip node=$name reason=unpinnable (no outpost.dhnt.io/host or kubernetes.io/hostname label)"
    continue
  fi

  say "candidate node=$name host=${nhost:-<none>} role=$role score=$score cpuFreeMilli=$free_cpu memFreeMiB=$((free_mem / 1048576)) siblingShards=$siblings usage=$usage_src"
  printf "%d${US}%d${US}%d${US}%s${US}%s${US}%s${US}%s${US}%s\n" \
    "$score" "$free_cpu" "$free_mem" "$name" "$nhost" "$pin_label" "$pin_value" "$usage_src" \
    >>"$candidates"
done <<EOF
$node_lines
EOF

if [ ! -s "$candidates" ]; then
  die 3 "no eligible node for backend=$BACKEND os=$WANT_OS arch=${WANT_ARCH:-any} host=${WANT_HOST:-any} needing cpu=$NEED_CPU_Q memory=$NEED_MEM_Q (considered $considered node(s); per-node reasons above)"
fi

# Total, deterministic order: score desc, cpu headroom desc, memory headroom
# desc, then node name ascending. The name key makes it a pure function of the
# input - identical fixtures always yield the identical node.
winner=$(sort -t "$US" -k1,1nr -k2,2nr -k3,3nr -k4,4 "$candidates" | head -n 1)

IFS="$US" read -r w_score w_cpu w_mem w_name w_host w_pin_label w_pin_value w_usage <<EOF
$winner
EOF

if [ -n "$DIAG_FILE" ]; then
  {
    printf 'node=%s\n' "$w_name"
    printf 'host=%s\n' "$w_host"
    printf 'pin_label=%s\n' "$w_pin_label"
    printf 'pin_value=%s\n' "$w_pin_value"
    printf 'score=%s\n' "$w_score"
    printf 'cpu_free_milli=%s\n' "$w_cpu"
    printf 'mem_free_bytes=%s\n' "$w_mem"
    printf 'usage_source=%s\n' "$w_usage"
    printf 'metrics_available=%s\n' "$metrics_available"
  } >"$DIAG_FILE"
fi

say "selected node=$w_name host=${w_host:-<none>} score=$w_score usage=$w_usage pin=$w_pin_label=$w_pin_value"
printf '%s\n' "$w_name"
