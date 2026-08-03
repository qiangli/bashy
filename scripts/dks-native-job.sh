#!/usr/bin/env bash
# Emit one native-platform DKS Job. vk-native materializes a checksum-verified
# Bashy release archive and executes it directly on the registered host OS.
#
# PLACEMENT. EXECUTOR_NODE selects where the shard runs and what the RunRecord
# records as its executor:
#
#   EXECUTOR_NODE=<name>   explicit, unchanged behavior: the name is recorded
#                          verbatim, no cluster is consulted, and the emitted
#                          nodeSelector is exactly what TARGET_OS/ARCH/HOST say.
#   EXECUTOR_NODE=auto     (the default) resource-aware placement via
#                          scripts/dks-select-node.sh: Ready, schedulable,
#                          label-matching nodes with real headroom for THIS
#                          task's requests, preferring idle capable workers and
#                          penalizing Dragon/control-plane so the coordination
#                          seat stays responsive. The chosen node is pinned into
#                          the nodeSelector, so the recorded executor and the
#                          actual placement cannot drift apart.
#
# `auto` is the default because the alternative default was "whatever the
# scheduler picks", which in practice meant Dragon. Selection FAILS CLOSED: no
# eligible node is a non-zero exit and no manifest, never a Job that quietly
# lands somewhere unsuitable.
#
# RESOURCE REQUESTS. Each task now carries realistic resources.requests (see
# task_defaults below), overridable with TASK_CPU/TASK_MEMORY, or removable
# entirely with TASK_RESOURCES=none — which restores the exact pre-placement
# manifest. Requests are what make sequential fan-out spread correctly: once a
# shard's Pod exists, the NEXT selection sees its request as reserved capacity
# even before the process starts consuming anything. See the CONCURRENCY section
# of scripts/dks-select-node.sh for what this does and does not guarantee.
set -euo pipefail

_here=$(cd "$(dirname "$0")" && pwd)

NAME="${NAME:-bashy-native}"
NS="${NS:-default}"
TARGET_OS="${TARGET_OS:?set TARGET_OS to linux, darwin, or windows}"
TARGET_ARCH="${TARGET_ARCH:-}"
TARGET_HOST="${TARGET_HOST:-}"
ARTIFACT_URL="${ARTIFACT_URL:?set ARTIFACT_URL to a published bashy release archive}"
ARTIFACT_SHA256="${ARTIFACT_SHA256:?set ARTIFACT_SHA256 to the archive checksum}"
ARTIFACT_PATH="${ARTIFACT_PATH:-bashy}"
TASK="${TASK:-smoke}"
SOURCE_URL="${SOURCE_URL:-https://github.com/qiangli/bashy.git}"
SOURCE_REF="${SOURCE_REF:?set SOURCE_REF to the exact source commit}"
SOURCE_SHA256="${SOURCE_SHA256:?set SOURCE_SHA256 to the source-tree sha256}"
PIPELINE_ID="${PIPELINE_ID:?set PIPELINE_ID to the dhnt.pipeline/v1 identity}"
EVIDENCE_TASK="${EVIDENCE_TASK:-$TASK}"
RUN_ID="${RUN_ID:-$NAME}"
TRACE_ID="${TRACE_ID:?set TRACE_ID to a 32-lowercase-hex trace ID}"
EXECUTOR_NODE="${EXECUTOR_NODE:-auto}"
TASK_RESOURCES="${TASK_RESOURCES:-requests}"
BASH53_TESTDATA_REPO="${BASH53_TESTDATA_REPO:-}"
BASH53_TESTDATA_REF="${BASH53_TESTDATA_REF:-}"
YASH_TESTDATA_REPO="${YASH_TESTDATA_REPO:-}"
YASH_TESTDATA_REF="${YASH_TESTDATA_REF:-}"
TTL="${TTL:-3600}"
BACKOFF="${BACKOFF:-1}"

case "$TARGET_OS" in linux|darwin|windows) ;; *)
  echo "dks-native-job: TARGET_OS must be linux, darwin, or windows" >&2
  exit 2
esac
case "$TASK" in smoke) ;; build|unit|bash53|yash)
  [ -n "$SOURCE_REF" ] || {
    echo "dks-native-job: SOURCE_REF is required for TASK=$TASK" >&2
    exit 2
  }
  ;; *)
  echo "dks-native-job: TASK must be smoke, build, unit, bash53, or yash" >&2
  exit 2
esac
if [ "$TASK" = bash53 ] && { [ -z "$BASH53_TESTDATA_REPO" ] || [ -z "$BASH53_TESTDATA_REF" ]; }; then
  echo "dks-native-job: BASH53_TESTDATA_REPO and BASH53_TESTDATA_REF are required for TASK=bash53" >&2
  exit 2
fi
if [ "$TASK" = bash53 ] && [ "$TARGET_OS" = windows ]; then
  echo "dks-native-job: TASK=bash53 requires a Unix vk-native host" >&2
  exit 2
fi
if [ "$TASK" = yash ] && { [ -z "$YASH_TESTDATA_REPO" ] || [ -z "$YASH_TESTDATA_REF" ]; }; then
  echo "dks-native-job: YASH_TESTDATA_REPO and YASH_TESTDATA_REF are required for TASK=yash" >&2
  exit 2
fi

case "$TASK_RESOURCES" in
  requests | none) ;;
  *)
    echo "dks-native-job: TASK_RESOURCES must be requests or none (got '$TASK_RESOURCES')" >&2
    exit 2
    ;;
esac
if [ "$EXECUTOR_NODE" = auto ] && [ "$TASK_RESOURCES" = none ]; then
  echo "dks-native-job: TASK_RESOURCES=none is incompatible with EXECUTOR_NODE=auto; without a Pod request, later shards cannot observe the selected reservation" >&2
  exit 2
fi

# Per-task requests. These are sized from what the task actually does, not from
# a uniform guess: a smoke is three subprocess launches, while the Bash 5.3
# suite runs 86 fixtures with a 4 GB per-fixture memory cap. A request that is
# too small is worse than none at all — it tells the selector a node has room it
# does not have.
case "$TASK" in
  smoke)
    default_cpu=250m
    default_mem=512Mi
    ;;
  build)
    default_cpu=2
    default_mem=4Gi
    ;;
  unit)
    default_cpu=2
    default_mem=4Gi
    ;;
  yash)
    default_cpu=2
    default_mem=4Gi
    ;;
  bash53)
    default_cpu=4
    default_mem=6Gi
    ;;
  *)
    default_cpu=1
    default_mem=2Gi
    ;;
esac
TASK_CPU="${TASK_CPU:-$default_cpu}"
TASK_MEMORY="${TASK_MEMORY:-$default_mem}"

selector_extra=""
[ -n "$TARGET_ARCH" ] && selector_extra="${selector_extra}
        kubernetes.io/arch: ${TARGET_ARCH}"
[ -n "$TARGET_HOST" ] && selector_extra="${selector_extra}
        outpost.dhnt.io/host: ${TARGET_HOST}"

# Resource-aware placement. Only on EXECUTOR_NODE=auto: an explicit node name is
# an operator decision and is recorded verbatim, exactly as before.
if [ "$EXECUTOR_NODE" = auto ]; then
  select_diag=$(mktemp)
  # No `VAR="${VAR:-$(cmd)}"` shorthand and no `|| exit $?` on an assignment:
  # both idioms have bitten these scripts under Bashy's expander (see
  # dks-profile.sh). Capture, then check, explicitly.
  set +e
  selected_node=$(
    DKS_SELECT_OS="$TARGET_OS" \
      DKS_SELECT_ARCH="$TARGET_ARCH" \
      DKS_SELECT_HOST="$TARGET_HOST" \
      DKS_SELECT_CPU="$TASK_CPU" \
      DKS_SELECT_MEM="$TASK_MEMORY" \
      DKS_SELECT_DIAG_FILE="$select_diag" \
      "$_here/dks-select-node.sh"
  )
  select_rc=$?
  set -e
  if [ "$select_rc" -ne 0 ] || [ -z "$selected_node" ]; then
    rm -f "$select_diag"
    echo "dks-native-job: EXECUTOR_NODE=auto found no eligible node for task=$TASK os=$TARGET_OS arch=${TARGET_ARCH:-any} host=${TARGET_HOST:-any} cpu=$TASK_CPU memory=$TASK_MEMORY (selector exit $select_rc; reasons above). Refusing to emit an unplaceable Job." >&2
    exit "$((select_rc == 0 ? 3 : select_rc))"
  fi
  # Pin the emitted Job to the node the evidence will name. Without this the
  # RunRecord would claim an executor the scheduler never had to honor — a
  # placement decision that reads as enforced and is not.
  pin_label=$(sed -n 's/^pin_label=//p' "$select_diag")
  pin_value=$(sed -n 's/^pin_value=//p' "$select_diag")
  rm -f "$select_diag"
  if [ -z "$pin_label" ] || [ -z "$pin_value" ]; then
    echo "dks-native-job: selector chose node '$selected_node' but reported no label to pin it with" >&2
    exit 3
  fi
  case "$selector_extra" in
    *"${pin_label}: ${pin_value}"*) ;; # already pinned by TARGET_HOST
    *)
      selector_extra="${selector_extra}
        ${pin_label}: ${pin_value}"
      ;;
  esac
  EXECUTOR_NODE="$selected_node"
  echo "dks-native-job: EXECUTOR_NODE=auto selected node=$EXECUTOR_NODE (pinned via ${pin_label}=${pin_value})" >&2
fi

resources_block=""
if [ "$TASK_RESOURCES" = requests ]; then
  resources_block="
          resources:
            requests:
              cpu: \"${TASK_CPU}\"
              memory: \"${TASK_MEMORY}\""
fi

cat <<YAML
apiVersion: batch/v1
kind: Job
metadata:
  name: ${NAME}
  namespace: ${NS}
  labels:
    app: ${NAME}
    dhnt.io/lane: native-platform
spec:
  completions: 1
  parallelism: 1
  backoffLimit: ${BACKOFF}
  ttlSecondsAfterFinished: ${TTL}
  template:
    metadata:
      labels:
        app: ${NAME}
        dhnt.io/lane: native-platform
      annotations:
        outpost.dhnt.io/termination-log-tail: "true"
        outpost.dhnt.io/native-artifact-url: "${ARTIFACT_URL}"
        outpost.dhnt.io/native-artifact-sha256: "${ARTIFACT_SHA256}"
        outpost.dhnt.io/native-artifact-path: "${ARTIFACT_PATH}"
    spec:
      restartPolicy: Never
      nodeSelector:
        outpost.dhnt.io/backend: vk-native
        kubernetes.io/os: ${TARGET_OS}${selector_extra}
      tolerations:
        - key: virtual-kubelet.io/provider
          operator: Equal
          value: outpost
          effect: NoSchedule
      containers:
        - name: smoke
          image: dhnt.io/native-process
          imagePullPolicy: Never${resources_block}
          command: ["bashy"]
          args:
            - "-c"
            - |
              set -e
              self="\$0"
              workspace=""
              failure_class=infra-fail
              started_at=\$("\$self" date -u '+%Y-%m-%dT%H:%M:%SZ')
              emit_result() {
                status=\$?
                trap - EXIT
                class=pass
                [ "\$status" -eq 0 ] || class="\$failure_class"
                case "\$status" in 130|143) class=canceled ;; esac
                finished_at=\$("\$self" date -u '+%Y-%m-%dT%H:%M:%SZ')
                record=\$("\$self" dhnt emit-run \
                  --pipeline "${PIPELINE_ID}" \
                  --task "${EVIDENCE_TASK}" \
                  --run "${RUN_ID}" \
                  --source-repository "${SOURCE_URL}" \
                  --source-commit "${SOURCE_REF}" \
                  --source-sha256 "${SOURCE_SHA256}" \
                  --input "candidate=${ARTIFACT_SHA256}" \
                  --node "${EXECUTOR_NODE}" \
                  --backend vk-native \
                  --os "\$os" \
                  --arch "\$arch" \
                  --class "\$class" \
                  --exit-code "\$status" \
                  --output "tested-candidate=${ARTIFACT_SHA256}" \
                  --started-at "\$started_at" \
                  --finished-at "\$finished_at" \
                  --trace-id "${TRACE_ID}") || status=70
                [ -z "\${record:-}" ] || printf 'DKS_RESULT:%s\n' "\$record"
                if [ -n "\$workspace" ]; then
                  if [ "\$os" = darwin ] || [ "\$os" = linux ]; then
                    /bin/chmod -R u+w "\$workspace" 2>/dev/null || true
                    /bin/rm -rf "\$workspace" 2>/dev/null || true
                  else
                    "\$self" chmod -R u+w "\$workspace" 2>/dev/null || true
                    "\$self" rm -rf "\$workspace" 2>/dev/null || true
                  fi
                fi
                exit "\$status"
              }
              trap emit_result EXIT
              os=\$(uname -s | tr A-Z a-z)
              arch=\$(uname -m)
              # Kubernetes and the release gate use Go's canonical platform
              # vocabulary. Native uname does not: Windows reports Windows_NT
              # and amd64 hosts commonly report x86_64.
              case "\$os" in
                windows_nt|mingw*|msys*) os=windows ;;
                darwin|linux) ;;
                *) echo "dks-native-job: unsupported observed OS \$os" >&2; exit 1 ;;
              esac
              case "\$arch" in
                x86_64|amd64) arch=amd64 ;;
                aarch64|arm64) arch=arm64 ;;
              esac
              [ "\$os" = "${TARGET_OS}" ] || {
                echo "dks-native-job: observed OS \$os does not match target ${TARGET_OS}" >&2
                exit 1
              }
              if [ -n "${TARGET_ARCH}" ] && [ "\$arch" != "${TARGET_ARCH}" ]; then
                echo "dks-native-job: observed arch \$arch does not match target ${TARGET_ARCH}" >&2
                exit 1
              fi
              failure_class=test-fail
              # Not '--version | head -1': a pipeline reports HEAD's status, so
              # a bashy that died here would have been read as a passing smoke.
              "\$self" --version >/dev/null
              [ "\$("\$self" -c 'echo runtime-ok')" = runtime-ok ]
              "\$self" curl --version >/dev/null
              task="${TASK}"
              if [ "\$task" != smoke ]; then
                # Staging the tree is infrastructure, not the system under test:
                # a clone that cannot reach the remote is an infra-fail, and
                # classifying it test-fail would blame the candidate for it.
                failure_class=infra-fail
                workspace=\$(mktemp -d)
                "\$self" git clone "${SOURCE_URL}" "\$workspace/bashy"
                "\$self" git -C "\$workspace/bashy" checkout --detach "${SOURCE_REF}"
                cd "\$workspace/bashy"
                BASHY="\$self" "\$self" scripts/bootstrap-siblings.sh
                failure_class=test-fail
                case "\$task" in
                  build)
                    BASHY="\$self" "\$self" dag dag.md build
                    ;;
                  unit)
                    "\$self" go test -short ./...
                    ;;
                  bash53)
                    BASHY="\$self" JOBS=1 BASH53_TESTDATA_REPO="${BASH53_TESTDATA_REPO}" \
                      BASH53_TESTDATA_REF="${BASH53_TESTDATA_REF}" \
                      "\$self" dag dag.md test-bash-parallel
                    ;;
                  yash)
                    "\$self" git clone "${YASH_TESTDATA_REPO}" .yash-tests
                    "\$self" git -C .yash-tests checkout --detach "${YASH_TESTDATA_REF}"
                    BASHY="\$self" "\$self" scripts/yash-scoreboard.sh
                    ;;
                esac
              fi
YAML
