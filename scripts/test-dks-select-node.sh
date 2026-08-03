#!/usr/bin/env bash
# Deterministic, offline tests for scripts/dks-select-node.sh and the
# EXECUTOR_NODE=auto wiring in scripts/dks-native-job.sh.
#
# FIXTURE JSON ONLY: every cluster read is served from test/dks-select/*.json.
# No cluster, no kubectl execution, no network, no clock, no randomness — so a
# failure here is a real behavior change, not a flaky environment.
#
# What each block proves, and why it is worth a test:
#   metrics            live usage is actually CONSUMED — the same node inventory
#                      picks a different node with metrics than without
#   no-metrics         the fallback WARNS and uses reserved requests; a missing
#                      reading is never silently scored as zero usage
#   partial metrics    one node missing from the metrics list warns by NAME, and
#                      REQUIRE_METRICS=1 turns that into a hard failure
#   exclusion          NotReady, cordoned, untolerated-taint, wrong
#                      backend/os/arch/host, and unpinnable nodes are all dropped
#   capacity           a node without room is dropped, and an empty candidate set
#                      is a non-zero exit with NO node name on stdout
#   ties               an exact tie resolves the same way every run
#   preferred          novicortex/novidesign beat a strictly roomier plain worker
#   dragon reserve     Dragon loses to any healthy worker, but is still usable
#                      when it is the only capacity — a penalty, not a ban
#   spread             a node already running a sibling shard loses to an idle one
#   explicit override  EXECUTOR_NODE=<name> consults NOTHING and records verbatim
#   secret non-leak    a credential-shaped KUBECTL/DKS_KUBECTL_ARGV never reaches
#                      any output stream, on success or on any failure path
#   peer fail-closed   DKS_PROFILE=peer with no kubeconfig exits non-zero and
#                      never silently retargets the cloudbox cluster
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
fx="$root/test/dks-select"
tmp=$(mktemp -d)
trap '/bin/rm -rf "$tmp"' EXIT
fail=0

note() { printf '%s\n' "$*" >&2; }
check() {
  local desc="$1" got="$2" want="$3"
  if [ "$got" != "$want" ]; then
    note "FAIL: $desc"
    note "  want: $want"
    note "  got:  $got"
    fail=1
  fi
}
contains() { # desc, haystack, needle
  case "$2" in
    *"$3"*) ;;
    *)
      note "FAIL: $1 (missing: $3)"
      fail=1
      ;;
  esac
}
lacks() { # desc, haystack, needle
  case "$2" in
    *"$3"*)
      note "FAIL: $1 (unexpectedly present: $3)"
      fail=1
      ;;
  esac
}

# Every run goes through a fresh subshell with an explicit environment. `env
# CMD` is deliberately NOT used: under bashy `env` is the pure-Go coreutils
# builtin, which does not run a command, so an `env`-based harness would fail
# on the very shell this repo ships.
#
# run_select "K=V" ... -> stdout (node name); stderr captured to $tmp/err;
# exit status readable as "$(last_rc)".
#
# `got=$(run_select ...)` puts the function body in a SUBSHELL, so the exit
# status cannot come back through a variable — an `rc=$?` inside it assigns a
# copy the parent never sees, and every fail-closed assertion then compares
# against a stale 0 and PASSES. That is precisely the "success reached by the
# absence of evidence" shape this repo keeps finding, so the status travels
# through a FILE the subshell writes and the parent reads back.
last_rc() { cat "$tmp/rc"; }
run_select() {
  local out
  set +e
  : >"$tmp/rc"
  out=$(
    cd "$root" || exit 99
    export DKS_SELECT_NODES_JSON="$fx/nodes.json"
    export DKS_SELECT_PODS_JSON="$fx/pods-empty.json"
    export DKS_SELECT_OS=darwin
    export DKS_SELECT_ARCH=arm64
    export DKS_SELECT_CPU=2
    export DKS_SELECT_MEM=4Gi
    for kv in "$@"; do export "$kv"; done
    bash scripts/dks-select-node.sh 2>"$tmp/err"
    inner_rc=$?
    printf '%s' "$inner_rc" >"$tmp/rc"
    exit "$inner_rc"
  )
  set -e
  printf '%s' "$out"
}

# ---------------------------------------------------------------------------
# 1. Baseline: requests-only scoring over the full inventory.

got=$(run_select)
check "baseline selects the lexicographically first tied preferred worker" "$got" novicortex
check "baseline exits 0" "$(last_rc)" 0
check "stdout is exactly one line" "$(printf '%s\n' "$got" | grep -c .)" 1
err=$(cat "$tmp/err")
contains "baseline pins the winner by its registered-host label" "$err" \
  "pin=outpost.dhnt.io/host=novicortex"

# 2. Metrics are actually consumed: same inventory, same pods, different answer.
#    novidesign is burning less than novicortex, so it wins once usage is
#    visible. If metrics were ignored this would still say novicortex.
got=$(run_select "DKS_SELECT_METRICS_JSON=$fx/metrics-idle.json")
check "live metrics change the selection" "$got" novidesign
err=$(cat "$tmp/err")
contains "metrics runs report usage=metrics" "$err" "usage=metrics"
lacks "metrics runs do not warn about unavailable metrics" "$err" \
  "metrics.k8s.io is unavailable"

# 2b. Live usage far above a node's (zero) requests removes it entirely. This is
#     the case requests-only scoring cannot see at all.
got=$(run_select "DKS_SELECT_METRICS_JSON=$fx/metrics-busy-novicortex.json")
check "a node busy beyond its requests is excluded" "$got" novidesign
err=$(cat "$tmp/err")
contains "the busy node is excluded for capacity, not filtered out" "$err" \
  "skip node=novicortex reason=insufficient-capacity"

# 3. No metrics: WARN and fall back to reserved requests. The warning is the
#    whole point — a silent fallback would make an overloaded node look idle.
got=$(run_select)
check "no-metrics fallback still selects" "$got" novicortex
err=$(cat "$tmp/err")
contains "no-metrics fallback warns" "$err" "metrics.k8s.io is unavailable"
contains "no-metrics warning says usage is not being treated as zero" "$err" \
  "not being treated as zero"
contains "no-metrics runs report usage=requests" "$err" "usage=requests"

# 3b. Reserved requests alone are enough to exclude a node.
got=$(run_select "DKS_SELECT_PODS_JSON=$fx/pods-busy-novicortex.json")
check "reserved requests exclude a node with no metrics" "$got" novidesign

# Effective requests are per Pod, not one global init-container maximum. Two
# init-heavy Pods plus overhead exceed novicortex even though either Pod alone
# fits; the historical global-max calculation incorrectly kept it eligible.
cat >"$tmp/pods-two-init.json" <<'EOF'
{"items":[
 {"metadata":{"uid":"p1","name":"p1","namespace":"default"},"spec":{"nodeName":"novicortex","containers":[{"resources":{"requests":{"cpu":"100m","memory":"64Mi"}}}],"initContainers":[{"resources":{"requests":{"cpu":"4","memory":"1Gi"}}}],"overhead":{"cpu":"200m","memory":"64Mi"}},"status":{"phase":"Pending"}},
 {"metadata":{"uid":"p2","name":"p2","namespace":"default"},"spec":{"nodeName":"novicortex","containers":[{"resources":{"requests":{"cpu":"100m","memory":"64Mi"}}}],"initContainers":[{"resources":{"requests":{"cpu":"4","memory":"1Gi"}}}],"overhead":{"cpu":"200m","memory":"64Mi"}},"status":{"phase":"Running"}}
]}
EOF
got=$(run_select "DKS_SELECT_PODS_JSON=$tmp/pods-two-init.json")
check "effective init requests are summed per Pod" "$got" novidesign
err=$(cat "$tmp/err")
contains "two init-heavy Pods exhaust novicortex" "$err" \
  "skip node=novicortex reason=insufficient-capacity"

# 3c. Terminal Pods hold nothing. A Succeeded/Failed Pod's requests must not
#     keep counting against a node forever.
got=$(run_select "DKS_SELECT_PODS_JSON=$fx/pods-finished-novicortex.json")
check "Succeeded/Failed pod requests are not counted as reserved" "$got" novicortex

# 3d. Partial metrics: the node MISSING from the list is named in a warning.
#     This is the sharpest edge of the whole design — a node absent from metrics
#     scores as if idle, so the operator has to be told which node that was.
got=$(run_select "DKS_SELECT_METRICS_JSON=$fx/metrics-partial.json")
check "partial metrics still selects" "$got" novidesign
err=$(cat "$tmp/err")
contains "partial metrics names the node with no reading" "$err" \
  "node=novidesign has no metrics.k8s.io reading"

# 3e. Strict mode turns every one of those into a hard failure.
got=$(run_select "DKS_SELECT_METRICS_JSON=$fx/metrics-partial.json" \
  DKS_SELECT_REQUIRE_METRICS=1)
check "REQUIRE_METRICS=1 with a missing per-node reading exits 6" "$(last_rc)" 6
check "REQUIRE_METRICS failure prints no node name" "$got" ""
got=$(run_select DKS_SELECT_REQUIRE_METRICS=1)
check "REQUIRE_METRICS=1 with no metrics at all exits 6" "$(last_rc)" 6
check "REQUIRE_METRICS total-absence failure prints no node name" "$got" ""

# 3f. Virtual nodes have no kubelet/NodeMetrics object. Their work still burns
# the registered host, so inherit utilization from the physical node carrying
# the same outpost host label. This is the live peer-DKS shape.
cat >"$tmp/nodes-virtual-host.json" <<'EOF'
{"items":[
 {"metadata":{"name":"worker-physical","labels":{"kubernetes.io/os":"linux","kubernetes.io/arch":"arm64","kubernetes.io/hostname":"worker-physical","outpost.dhnt.io/backend":"k3s","outpost.dhnt.io/host":"novidesign","outpost.dhnt.io/runtime":"agent"}},"spec":{},"status":{"allocatable":{"cpu":"8","memory":"16Gi"},"conditions":[{"type":"Ready","status":"True"}]}},
 {"metadata":{"name":"worker-vk-native","labels":{"kubernetes.io/os":"darwin","kubernetes.io/arch":"arm64","kubernetes.io/hostname":"worker-vk-native","outpost.dhnt.io/backend":"vk-native","outpost.dhnt.io/host":"novidesign","outpost.dhnt.io/runtime":"virtual"}},"spec":{},"status":{"allocatable":{"cpu":"14","memory":"36Gi"},"conditions":[{"type":"Ready","status":"True"}]}}
]}
EOF
cat >"$tmp/metrics-physical-busy.json" <<'EOF'
{"items":[{"metadata":{"name":"worker-physical"},"usage":{"cpu":"13","memory":"4Gi"}}]}
EOF
got=$(run_select "DKS_SELECT_NODES_JSON=$tmp/nodes-virtual-host.json" \
  "DKS_SELECT_METRICS_JSON=$tmp/metrics-physical-busy.json")
check "host utilization can exclude an overloaded virtual node" "$(last_rc)" 3
check "an overloaded virtual node emits no selection" "$got" ""
err=$(cat "$tmp/err")
contains "virtual-node capacity names its physical-host metrics source" "$err" \
  "usage=host-metrics"

cat >"$tmp/metrics-physical-idle.json" <<'EOF'
{"items":[{"metadata":{"name":"worker-physical"},"usage":{"cpu":"1","memory":"2Gi"}}]}
EOF
got=$(run_select "DKS_SELECT_NODES_JSON=$tmp/nodes-virtual-host.json" \
  "DKS_SELECT_METRICS_JSON=$tmp/metrics-physical-idle.json" \
  DKS_SELECT_REQUIRE_METRICS=1)
check "strict metrics accepts a virtual node backed by physical-host metrics" "$got" \
  worker-vk-native
err=$(cat "$tmp/err")
contains "virtual selection reports host metrics" "$err" "usage=host-metrics"

# ---------------------------------------------------------------------------
# 4. Schedulability exclusions. Each of these nodes is the roomiest in the
#    inventory (32 CPU / 128Gi) precisely so that "it did not win" can only mean
#    "it was excluded", never "it lost on score".

got=$(run_select)
err=$(cat "$tmp/err")
contains "a NotReady node is excluded" "$err" "skip node=novistale reason=not-ready"
contains "a cordoned node is excluded" "$err" "skip node=novicold reason=cordoned"
contains "an untolerated taint excludes a node" "$err" \
  "skip node=novilocked reason=untolerated-taint"
contains "a wrong-arch node is excluded" "$err" "skip node=novix86 reason=arch-mismatch"
contains "a wrong-backend node is excluded" "$err" "skip node=novipod reason=backend-mismatch"
contains "a wrong-os node is excluded" "$err" "skip node=lern reason=os-mismatch"
contains "a too-small node is excluded" "$err" \
  "skip node=novitiny reason=insufficient-capacity"

# 4b. Host pinning narrows to exactly one registered host, and still enforces
#     capacity rather than trusting the pin.
got=$(run_select DKS_SELECT_HOST=novidesign)
check "an explicit host constraint selects that host's node" "$got" novidesign
got=$(run_select DKS_SELECT_HOST=novitiny)
check "an explicit host with no room fails closed" "$(last_rc)" 3
check "an explicit host with no room prints no node name" "$got" ""

# 4c. Other operating systems resolve to their own nodes, not to a darwin one.
got=$(run_select DKS_SELECT_OS=linux DKS_SELECT_ARCH=amd64)
check "linux selects the linux node" "$got" lern
got=$(run_select DKS_SELECT_OS=windows DKS_SELECT_ARCH=amd64)
check "windows selects the windows node" "$got" puppy-windows

# 4d. A node that cannot be pinned by any label is not selectable: emitting it
#     would produce a RunRecord naming an executor the Job never had to honor.
got=$(run_select "DKS_SELECT_NODES_JSON=$fx/nodes-unpinnable.json")
check "an unpinnable node is skipped in favor of a pinnable one" "$got" novihostname
err=$(cat "$tmp/err")
contains "the unpinnable node says so" "$err" "skip node=noviorphan reason=unpinnable"
contains "a hostname-only node pins by kubernetes.io/hostname" "$err" \
  "pin=kubernetes.io/hostname=novihostname"

# ---------------------------------------------------------------------------
# 5. Capacity insufficiency across the whole fleet is a fail-closed exit, not a
#    best-effort guess.

got=$(run_select DKS_SELECT_CPU=64 DKS_SELECT_MEM=512Gi)
check "a request nothing can hold exits 3" "$(last_rc)" 3
check "an unsatisfiable request prints no node name" "$got" ""
err=$(cat "$tmp/err")
contains "the unsatisfiable-request error names the requirement" "$err" "no eligible node"

got=$(run_select "DKS_SELECT_NODES_JSON=$fx/nodes-empty.json")
check "an empty node inventory exits 3" "$(last_rc)" 3
check "an empty node inventory prints no node name" "$got" ""

# ---------------------------------------------------------------------------
# 6. Determinism. novicortex and novidesign are byte-identical in the fixture, so
#    this is a genuine tie broken only by the final name key.

err=$(cat "$tmp/err")
got=$(run_select)
err=$(cat "$tmp/err")
contains "novicortex and novidesign tie on score" "$err" "node=novicortex host=novicortex role=preferred score=100"
contains "novidesign scores identically" "$err" "node=novidesign host=novidesign role=preferred score=100"
ties=""
i=0
while [ "$i" -lt 5 ]; do
  ties="$ties $(run_select)"
  i=$((i + 1))
done
check "an exact tie resolves identically on every run" "$ties" \
  " novicortex novicortex novicortex novicortex novicortex"

# Determinism must not depend on the operator's collation order either.
got=$(run_select LC_ALL=en_US.UTF-8)
check "tie-breaking is locale-independent" "$got" novicortex

# ---------------------------------------------------------------------------
# 7. Preference and reserve.

# noviextra has strictly more free CPU and memory than either preferred host and
# is still not chosen: the preference bonus is what makes this test meaningful.
got=$(run_select)
check "a preferred host beats a roomier plain worker" "$got" novicortex
got=$(run_select DKS_SELECT_PREFERRED_HOSTS=)
check "with no preferred hosts the roomiest worker wins" "$got" noviextra
got=$(run_select DKS_SELECT_PREFERRED_HOSTS=noviextra)
check "the preferred-host list is configurable" "$got" noviextra

# Dragon is the coordination seat: penalized, never chosen while a healthy
# worker exists...
got=$(run_select)
lacks "dragon is not selected while workers are available" "$got" dragon
err=$(cat "$tmp/err")
contains "dragon is scored as reserved" "$err" "node=dragon host=dragon role=reserved"

# ...but a penalty is not a ban. When Dragon is the only capacity, refusing to
# run at all would be worse than running on it.
got=$(run_select "DKS_SELECT_NODES_JSON=$fx/nodes-dragon-only.json")
check "dragon is still usable when it is the only capacity" "$got" dragon
check "dragon-only selection exits 0" "$(last_rc)" 0

# The reserve list is configurable, and reserving the preferred hosts moves the
# choice off them.
got=$(run_select DKS_SELECT_RESERVED_HOSTS=novicortex,novidesign)
check "the reserved-host list is configurable" "$got" noviextra

# A control-plane node is reserved by role, without being named in any list.
got=$(run_select "DKS_SELECT_NODES_JSON=$fx/nodes-controlplane.json")
check "a control-plane node loses to a plain worker" "$got" noviplain
err=$(cat "$tmp/err")
contains "the control-plane node is scored as reserved" "$err" "node=novictl"

# ---------------------------------------------------------------------------
# 8. Shard spreading. A node already running a sibling shard loses to an idle
#    peer even though it still has ample room — this is what keeps a fan-out
#    from stacking on one host.

got=$(run_select "DKS_SELECT_PODS_JSON=$fx/pods-one-shard-novicortex.json")
check "a node already running a shard loses to an idle peer" "$got" novidesign
err=$(cat "$tmp/err")
contains "the sibling shard is counted" "$err" "siblingShards=1"

# That shard also RESERVES 250m/512Mi, so the case above cannot tell the spread
# penalty apart from the plain loss of headroom — novidesign would win either
# way. Two tests pull them apart.
#
# First, on the SAME fixture, disabling the penalty must NOT flip the winner:
# novicortex genuinely has less room, so novidesign still wins on headroom. What
# changes is only novicortex's score, by exactly the penalty.
got=$(run_select "DKS_SELECT_PODS_JSON=$fx/pods-one-shard-novicortex.json" \
  DKS_SELECT_SPREAD_PENALTY=0)
check "disabling the penalty does not undo a real reservation" "$got" novidesign
err=$(cat "$tmp/err")
contains "with the penalty off the sibling costs nothing" "$err" \
  "node=novicortex host=novicortex role=preferred score=96"
got=$(run_select "DKS_SELECT_PODS_JSON=$fx/pods-one-shard-novicortex.json")
err=$(cat "$tmp/err")
contains "with the penalty on the sibling costs exactly 35" "$err" \
  "node=novicortex host=novicortex role=preferred score=61"

# Second, a sibling that reserves NOTHING isolates the penalty completely: the
# two preferred workers are then identical on every capacity dimension, so the
# sibling count is the only thing that can separate them. This is the case that
# actually proves the penalty spreads a fan-out.
got=$(run_select "DKS_SELECT_PODS_JSON=$fx/pods-zero-request-shard-novicortex.json")
check "a zero-request sibling still pushes the next shard to an idle peer" "$got" novidesign
got=$(run_select "DKS_SELECT_PODS_JSON=$fx/pods-zero-request-shard-novicortex.json" \
  DKS_SELECT_SPREAD_PENALTY=0)
check "the spread penalty is configurable" "$got" novicortex

# The documented concurrent-fan-out workaround: exclude what you already placed.
got=$(run_select DKS_SELECT_EXCLUDE_NODES=novicortex)
check "an excluded node is skipped" "$got" novidesign
got=$(run_select DKS_SELECT_EXCLUDE_NODES=novicortex,novidesign)
check "an exclusion list is honored in full" "$got" noviextra
err=$(cat "$tmp/err")
contains "exclusions are reported" "$err" "skip node=novicortex reason=explicitly-excluded"

# ---------------------------------------------------------------------------
# 9. Malformed input fails closed rather than scoring as zero. A quantity that
#    parsed as 0 would make a node look either infinitely idle or fatally small,
#    and both are wrong answers that look like working answers.

printf 'not json at all' >"$tmp/bad.json"
got=$(run_select "DKS_SELECT_NODES_JSON=$tmp/bad.json")
check "a malformed node payload exits 5" "$(last_rc)" 5
check "a malformed node payload prints no node name" "$got" ""
got=$(run_select "DKS_SELECT_NODES_JSON=$tmp/does-not-exist.json")
check "an unreadable node fixture exits 5" "$(last_rc)" 5

cat >"$tmp/nodes-badqty.json" <<'EOF'
{"items":[{"metadata":{"name":"weird","labels":{"kubernetes.io/os":"darwin","kubernetes.io/arch":"arm64","kubernetes.io/hostname":"weird","outpost.dhnt.io/backend":"vk-native","outpost.dhnt.io/host":"weird"}},"spec":{},"status":{"allocatable":{"cpu":"eight","memory":"16Gi"},"conditions":[{"type":"Ready","status":"True"}]}}]}
EOF
got=$(run_select "DKS_SELECT_NODES_JSON=$tmp/nodes-badqty.json")
check "an unparseable quantity exits 2 rather than scoring as zero" "$(last_rc)" 2
check "an unparseable quantity prints no node name" "$got" ""
err=$(cat "$tmp/err")
contains "the unparseable quantity is named" "$err" "unparseable"

# ---------------------------------------------------------------------------
# 10. Quantity vocabulary. Kubernetes does not speak one unit: allocatable
#     arrives as "8" or "7900m", metrics CPU in nanocores, metrics memory in Ki.
#     A node whose CPU is expressed in millicores must be usable.

got=$(run_select DKS_SELECT_OS=windows DKS_SELECT_ARCH=amd64 DKS_SELECT_CPU=7500m \
  DKS_SELECT_MEM=8Gi)
check "millicore allocatable is parsed (7900m holds 7500m)" "$got" puppy-windows
got=$(run_select DKS_SELECT_OS=windows DKS_SELECT_ARCH=amd64 DKS_SELECT_CPU=7950m \
  DKS_SELECT_MEM=8Gi)
check "millicore allocatable is not rounded up (7900m cannot hold 7950m)" "$(last_rc)" 3

# ---------------------------------------------------------------------------
# 11. dks-native-job.sh wiring.

emit_job() { # "K=V"... -> stdout YAML; stderr to $tmp/joberr; status via last_rc
  local out
  set +e
  : >"$tmp/rc"
  out=$(
    cd "$root" || exit 99
    export TARGET_OS=darwin
    export TARGET_ARCH=arm64
    export ARTIFACT_URL=https://example.invalid/bashy.tar.gz
    export ARTIFACT_SHA256=1111111111111111111111111111111111111111111111111111111111111111
    export SOURCE_REF=2222222222222222222222222222222222222222
    export SOURCE_SHA256=3333333333333333333333333333333333333333333333333333333333333333
    export PIPELINE_ID=dhnt.pipeline/test
    export TRACE_ID=44444444444444444444444444444444
    export TASK=smoke
    for kv in "$@"; do export "$kv"; done
    bash scripts/dks-native-job.sh 2>"$tmp/joberr"
    inner_rc=$?
    printf '%s' "$inner_rc" >"$tmp/rc"
    exit "$inner_rc"
  )
  set -e
  printf '%s' "$out"
}

# 11a. EXPLICIT override: the node name is recorded verbatim and NOTHING is
#      consulted to get it. Proven by pointing the fixtures at an EMPTY node
#      inventory — if the selector ran at all, this would fail closed.
yaml=$(emit_job EXECUTOR_NODE=hand-picked-node \
  "DKS_SELECT_NODES_JSON=$fx/nodes-empty.json" \
  "DKS_SELECT_PODS_JSON=$fx/pods-empty.json")
check "an explicit EXECUTOR_NODE emits successfully" "$(last_rc)" 0
contains "an explicit EXECUTOR_NODE is recorded verbatim" "$yaml" \
  '--node "hand-picked-node"'
lacks "an explicit EXECUTOR_NODE adds no host pin" "$yaml" "outpost.dhnt.io/host:"
joberr=$(cat "$tmp/joberr")
lacks "an explicit EXECUTOR_NODE never runs the selector" "$joberr" "dks-select-node"

# 11b. AUTO: selects, records what it selected, and pins the Job to it. The
#      recorded executor and the emitted nodeSelector must agree — a RunRecord
#      naming a node the scheduler was free to ignore is not evidence.
yaml=$(emit_job EXECUTOR_NODE=auto \
  "DKS_SELECT_NODES_JSON=$fx/nodes.json" \
  "DKS_SELECT_PODS_JSON=$fx/pods-empty.json")
check "EXECUTOR_NODE=auto emits successfully" "$(last_rc)" 0
contains "auto records the selected node" "$yaml" '--node "novicortex"'
contains "auto pins the Job to the selected node" "$yaml" \
  "outpost.dhnt.io/host: novicortex"
joberr=$(cat "$tmp/joberr")
contains "auto reports its choice" "$joberr" "selected node=novicortex"

# 11c. An UNSET EXECUTOR_NODE defaults to auto (it used to be a hard require).
yaml=$(emit_job "DKS_SELECT_NODES_JSON=$fx/nodes.json" \
  "DKS_SELECT_PODS_JSON=$fx/pods-empty.json")
check "an unset EXECUTOR_NODE defaults to auto" "$(last_rc)" 0
contains "the default auto path records a selected node" "$yaml" '--node "novicortex"'

# 11d. Auto fails closed: no eligible node means no manifest at all. A partial
#      or unplaced Job would be applied and then wait forever.
yaml=$(emit_job EXECUTOR_NODE=auto \
  "DKS_SELECT_NODES_JSON=$fx/nodes-empty.json" \
  "DKS_SELECT_PODS_JSON=$fx/pods-empty.json")
[ "$(last_rc)" -ne 0 ] || {
  note "FAIL: auto with no eligible node exited 0"
  fail=1
}
check "auto with no eligible node emits no manifest" "$yaml" ""
joberr=$(cat "$tmp/joberr")
contains "auto's failure explains itself" "$joberr" "found no eligible node"

# 11e. Per-task requests, and their overrides. bash53 asks for materially more
#      than a smoke because it does materially more.
yaml=$(emit_job EXECUTOR_NODE=pinned TASK=smoke)
contains "smoke carries a small CPU request" "$yaml" 'cpu: "250m"'
contains "smoke carries a small memory request" "$yaml" 'memory: "512Mi"'

yaml=$(emit_job EXECUTOR_NODE=pinned TASK=bash53 BASH53_TESTDATA_REPO=r \
  BASH53_TESTDATA_REF=v1)
contains "bash53 carries a large CPU request" "$yaml" 'cpu: "4"'
contains "bash53 carries a large memory request" "$yaml" 'memory: "6Gi"'

yaml=$(emit_job EXECUTOR_NODE=pinned TASK=build TASK_CPU=6 TASK_MEMORY=12Gi)
contains "TASK_CPU overrides the default" "$yaml" 'cpu: "6"'
contains "TASK_MEMORY overrides the default" "$yaml" 'memory: "12Gi"'

yaml=$(emit_job EXECUTOR_NODE=pinned TASK=build TASK_RESOURCES=none)
lacks "TASK_RESOURCES=none emits no resources block" "$yaml" "resources:"

yaml=$(emit_job EXECUTOR_NODE=auto TASK=build TASK_RESOURCES=none \
  "DKS_SELECT_NODES_JSON=$fx/nodes.json" \
  "DKS_SELECT_PODS_JSON=$fx/pods-empty.json")
check "auto placement refuses an invisible zero-request reservation" "$(last_rc)" 2
check "the refused auto/none combination emits no manifest" "$yaml" ""

# 11f. The request drives the selection: asking for more than a preferred worker
#      has moves the shard to the node that can actually hold it.
yaml=$(emit_job EXECUTOR_NODE=auto TASK=build TASK_CPU=12 TASK_MEMORY=32Gi \
  "DKS_SELECT_NODES_JSON=$fx/nodes.json" \
  "DKS_SELECT_PODS_JSON=$fx/pods-empty.json")
check "a large request emits successfully" "$(last_rc)" 0
contains "a request larger than the preferred workers moves to the big node" \
  "$yaml" '--node "noviextra"'

# ---------------------------------------------------------------------------
# 12. Scheduler-native placement for concurrent Indexed Jobs. These Pods must
#     remain free for Kubernetes to spread independently; hard-pinning every
#     completion to the selector's one snapshot winner would defeat fan-out.

emit_indexed() { # "K=V"... -> stdout YAML; stderr to $tmp/indexederr
  local out
  set +e
  : >"$tmp/rc"
  out=$(
    cd "$root" || exit 99
    export IMAGE=example.invalid/bashy:test
    for kv in "$@"; do export "$kv"; done
    bash scripts/dag-to-k8s-job.sh 2>"$tmp/indexederr"
    inner_rc=$?
    printf '%s' "$inner_rc" >"$tmp/rc"
    exit "$inner_rc"
  )
  set -e
  printf '%s' "$out"
}

yaml=$(emit_indexed VENUE=agent)
check "agent Indexed Job emits successfully" "$(last_rc)" 0
contains "agent Jobs require a runtime-ready physical worker" "$yaml" \
  'outpost.dhnt.io/runtime-ready: "true"'
contains "Indexed Jobs prefer novicortex and novidesign" "$yaml" \
  'values: ["novicortex", "novidesign"]'
contains "Indexed Jobs reserve Dragon without banning it" "$yaml" \
  'operator: NotIn'
contains "Indexed Jobs name Dragon in the reserve preference" "$yaml" \
  'values: ["dragon"]'
contains "Indexed Jobs avoid control-plane nodes when workers exist" "$yaml" \
  'key: node-role.kubernetes.io/control-plane'
contains "Indexed Jobs spread by physical hostname" "$yaml" \
  'topologyKey: kubernetes.io/hostname'
lacks "Indexed Jobs are not hard-pinned to a preferred host" "$yaml" \
  'outpost.dhnt.io/host: novicortex'

yaml=$(emit_indexed VENUE=vk-podman)
check "vk-podman Indexed Job emits successfully" "$(last_rc)" 0
contains "vk-podman keeps its backend selector" "$yaml" \
  'outpost.dhnt.io/backend: vk-podman'
contains "vk-podman keeps its virtual-kubelet toleration" "$yaml" \
  'key: virtual-kubelet.io/provider'
contains "vk-podman also receives topology spreading" "$yaml" \
  'topologySpreadConstraints:'

yaml=$(emit_indexed DKS_PREFERRED_HOSTS='novicortex,not valid')
check "an invalid preferred-host list fails closed" "$(last_rc)" 2
check "an invalid preferred-host list emits no manifest" "$yaml" ""
yaml=$(emit_indexed DKS_RESERVED_HOSTS='dragon,,other')
check "an empty host-list element fails closed" "$(last_rc)" 2
check "an empty host-list element emits no manifest" "$yaml" ""

# ---------------------------------------------------------------------------
# 13. SECRET NON-LEAK. A kubectl argv can carry a token; this session may be
#     captured to disk unredacted. The secret must appear in NO output stream on
#     ANY path — fixture path, live-query failure path, or job-emitter path.

secret='SUPERSECRETtok3n'
bindir="$tmp/bin"
mkdir -p "$bindir"
# A SILENT fake kubectl: it never echoes its argv, so any appearance of the
# secret in the output is these scripts leaking it, not the fake.
cat >"$bindir/kubectl-silent" <<'EOF'
#!/usr/bin/env bash
exit 7
EOF
chmod +x "$bindir/kubectl-silent"
silent="$bindir/kubectl-silent"

leak_run() { # desc, "K=V"...
  local desc="$1" out
  shift
  set +e
  out=$(
    cd "$root" || exit 99
    export DKS_SELECT_OS=darwin
    export DKS_SELECT_CPU=2
    export DKS_SELECT_MEM=4Gi
    for kv in "$@"; do export "$kv"; done
    bash scripts/dks-select-node.sh 2>&1
  )
  set -e
  lacks "$desc leaked the override secret" "$out" "$secret"
}

for carrier in KUBECTL ARGV; do
  if [ "$carrier" = KUBECTL ]; then
    cred="KUBECTL=$silent --token=$secret"
  else
    cred="DKS_KUBECTL_ARGV=$silent
--token=$secret"
  fi
  # Fixture path (succeeds) and live path (every kubectl call fails).
  leak_run "selector fixture path ($carrier)" "$cred" \
    "DKS_SELECT_NODES_JSON=$fx/nodes.json" "DKS_SELECT_PODS_JSON=$fx/pods-empty.json"
  leak_run "selector live-query failure path ($carrier)" "$cred"
  # Failure-to-select path, which prints the longest diagnostic of any path.
  leak_run "selector no-candidate path ($carrier)" "$cred" \
    "DKS_SELECT_NODES_JSON=$fx/nodes-empty.json" "DKS_SELECT_PODS_JSON=$fx/pods-empty.json"
done

# The job emitter's auto path is a fourth consumer: it shells out to the
# selector, so a leak could surface through it too.
set +e
out=$(
  cd "$root" || exit 99
  export TARGET_OS=darwin TARGET_ARCH=arm64
  export ARTIFACT_URL=https://example.invalid/b.tgz ARTIFACT_SHA256=aa
  export SOURCE_REF=bb SOURCE_SHA256=cc PIPELINE_ID=p TRACE_ID=dd TASK=smoke
  export EXECUTOR_NODE=auto
  export KUBECTL="$silent --token=$secret"
  bash scripts/dks-native-job.sh 2>&1
)
set -e
lacks "dks-native-job.sh auto path leaked the override secret" "$out" "$secret"

# ---------------------------------------------------------------------------
# 14. PEER FAIL-CLOSED. DKS_PROFILE=peer with a missing kubeconfig must exit
#     non-zero naming the path, and must never quietly retarget cloudbox —
#     selecting a node from the WRONG cluster is the failure this prevents.

missing_kc="$tmp/no-such-kubeconfig.yaml"
got=$(run_select DKS_PROFILE=peer "DKS_PEER_KUBECONFIG=$missing_kc")
check "peer with a missing kubeconfig fails closed" "$(last_rc)" 9
check "peer with a missing kubeconfig prints no node name" "$got" ""
err=$(cat "$tmp/err")
contains "the peer failure names the missing path" "$err" "$missing_kc"
lacks "the peer failure does not announce a cloudbox fallback" "$err" "profile=cloudbox"

# An explicitly-empty DKS_PROFILE is a typo'd variable, not "unset".
got=$(run_select DKS_PROFILE=)
check "an empty DKS_PROFILE fails closed" "$(last_rc)" 9
check "an empty DKS_PROFILE prints no node name" "$got" ""

got=$(run_select DKS_PROFILE=cloudbox)
check "an explicit cloudbox profile is refused" "$(last_rc)" 9
check "a refused cloudbox profile prints no node name" "$got" ""
err=$(cat "$tmp/err")
contains "the cloudbox refusal names peer as required" "$err" \
  "requires DKS_PROFILE=peer"

# ---------------------------------------------------------------------------
# 15. Bashy regression. These scripts must run under the shell this repo ships,
#     not just under GNU bash — and bashy carries its OWN pure-Go jq, so every
#     jq construct here (env.VAR, map(tostring), join("")) has to work on
#     both implementations. Fixtures keep it offline.

run_under_bashy() { # desc, want, "K=V"...
  local desc="$1" want="$2" out
  shift 2
  set +e
  out=$(
    cd "$root" || exit 99
    export DKS_SELECT_PODS_JSON="$fx/pods-empty.json"
    export DKS_SELECT_OS=darwin DKS_SELECT_ARCH=arm64
    export DKS_SELECT_CPU=2 DKS_SELECT_MEM=4Gi
    for kv in "$@"; do export "$kv"; done
    "$bashy_bin" scripts/dks-select-node.sh 2>&1
  )
  set -e
  case "$out" in
    *'panic:'* | *'SIGSEGV'* | *'goroutine '*)
      note "FAIL: $desc panicked under bashy"
      note "$out"
      fail=1
      return
      ;;
  esac
  contains "$desc" "$out" "$want"
}

if command -v bashy >/dev/null 2>&1; then
  bashy_bin="$(command -v bashy)"
  run_under_bashy "bashy: metrics-driven selection" "selected node=novidesign" \
    "DKS_SELECT_NODES_JSON=$fx/nodes.json" \
    "DKS_SELECT_METRICS_JSON=$fx/metrics-idle.json"

  # The separator regression, under bashy's own jq. These nodes have EMPTY
  # host/hostname labels, which is what a tab-delimited record silently
  # collapsed — shifting allocatable CPU into the Ready slot and reporting a
  # healthy node as `not-ready condition=32`. If bashy's jq rendered the
  # separator differently, this is where it would show.
  run_under_bashy "bashy: empty label fields do not shift columns" \
    "selected node=novihostname" "DKS_SELECT_NODES_JSON=$fx/nodes-unpinnable.json"
  run_under_bashy "bashy: a hostname-only node still pins correctly" \
    "pin=kubernetes.io/hostname=novihostname" \
    "DKS_SELECT_NODES_JSON=$fx/nodes-unpinnable.json"
  run_under_bashy "bashy: an unpinnable node is reported as such" \
    "skip node=noviorphan reason=unpinnable" \
    "DKS_SELECT_NODES_JSON=$fx/nodes-unpinnable.json"

  # Pod-request summing and the spread count are separate jq programs.
  run_under_bashy "bashy: reserved pod requests are summed" \
    "selected node=novidesign" "DKS_SELECT_NODES_JSON=$fx/nodes.json" \
    "DKS_SELECT_PODS_JSON=$fx/pods-busy-novicortex.json"
  run_under_bashy "bashy: sibling shards are counted" "siblingShards=1" \
    "DKS_SELECT_NODES_JSON=$fx/nodes.json" \
    "DKS_SELECT_PODS_JSON=$fx/pods-one-shard-novicortex.json"

  # Fail-closed paths must fail closed on bashy too, not just on GNU bash. An
  # empty inventory and a fleet-wide capacity shortfall are DIFFERENT refusals,
  # so each is asserted on its own message rather than on "it exited non-zero".
  run_under_bashy "bashy: an empty inventory fails closed" \
    "the node inventory contains no nodes at all" \
    "DKS_SELECT_NODES_JSON=$fx/nodes-empty.json"
  run_under_bashy "bashy: an unsatisfiable request fails closed" "no eligible node" \
    "DKS_SELECT_NODES_JSON=$fx/nodes.json" \
    DKS_SELECT_CPU=64 DKS_SELECT_MEM=512Gi
else
  note "SKIP: bashy not installed - bashy regression not run"
fi

if [ "$fail" -eq 0 ]; then
  echo "dks node selection: PASS"
else
  echo "dks node selection: FAIL" >&2
  exit 1
fi
