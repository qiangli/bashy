#!/usr/bin/env bash
# Emit a chunked conformance run as a native Kubernetes (DKS) Indexed Job.
#
# This is the tier-5 "B path": plain `batch/v1` Job, no Argo controller
# required (see scripts/dag-to-argo.sh for the "A path" that lowers the same
# run onto Argo Workflows). One chunk becomes one pod via completionMode:
# Indexed — k8s injects JOB_COMPLETION_INDEX, the container turns that into a
# CHUNK=i/N. The scheduler places pods on whatever node is Ready and reschedules
# them off a lost/drained node, which is the whole reason to run on the cluster:
# a host drop costs duration, never results (the "retry infra, keep results"
# contract — backoffLimit retries pod/node failures; a non-zero TEST result is a
# pod success that the aggregator reads out of the logs).
#
# The image is expected to be SIDE-LOADED into each node's containerd
# (imagePullPolicy: Never) — a self-contained, SQUASHED single-layer image built
# by scripts/build-conformance-image.sh with SQUASH=1 (a multi-layer image fails
# `ctr images import` on a nested k3s-in-podman node: overlayfs whiteouts need
# CAP_MKNOD the nesting denies). Side-load with, e.g.:
#
#   podman save <IMAGE> | ssh <node-host> \
#     'podman exec -i <node-runtime> k3s ctr -n k8s.io images import -'
#
# Usage:
#   dag-to-k8s-job.sh > job.yaml
#   NAME=yash NS=user-abc IMAGE=localhost/yash-k8s:arm64-squash CHUNKS=12 \
#     SUITE_CMD='./run-yash -chunk ${CHUNK}' dag-to-k8s-job.sh | kubectl apply -f -
#
# Every knob is an env var / flag — nothing about a particular host, cluster, or
# corpus is baked in (bashy is a general tool; a committed script must carry no
# operator's private topology).
set -euo pipefail

NAME="${NAME:-bash53-conformance}"
NS="${NS:-default}"
IMAGE="${IMAGE:?set IMAGE to the side-loaded, squashed conformance image}"
CHUNKS="${CHUNKS:-8}"                    # corpus property (chunks.json), NOT fleet size
PARALLELISM="${PARALLELISM:-4}"          # how many pods run at once
ARCH="${ARCH:-arm64}"
BACKOFF="${BACKOFF:-8}"                  # retry budget for INFRA failures
TTL="${TTL:-3600}"
REQ_CPU="${REQ_CPU:-500m}"
REQ_MEM="${REQ_MEM:-512Mi}"
LIM_MEM="${LIM_MEM:-2Gi}"

# The per-chunk command. The literal ${CHUNK} is expanded IN THE POD (from
# JOB_COMPLETION_INDEX), so it is single-quoted here to survive both this script
# and the heredoc unchanged. Set with a plain if — NOT a ${SUITE_CMD:-…} default —
# because nesting ${CHUNK} inside a ${var:-…} expansion breaks the brace parser.
if [ -z "${SUITE_CMD:-}" ]; then
  SUITE_CMD='./bin/bash53suite-linux-'"${ARCH}"' -tests-dir /bash53/tests -chunk ${CHUNK} -bash ./bin/bash-linux-'"${ARCH}"'/bash'
fi

cat <<YAML
apiVersion: batch/v1
kind: Job
metadata:
  name: ${NAME}
  namespace: ${NS}
  labels: { app: ${NAME} }
spec:
  completions: ${CHUNKS}
  parallelism: ${PARALLELISM}
  completionMode: Indexed
  backoffLimit: ${BACKOFF}
  ttlSecondsAfterFinished: ${TTL}
  template:
    metadata:
      labels: { app: ${NAME} }
    spec:
      restartPolicy: Never
      containers:
        - name: chunk
          image: ${IMAGE}
          imagePullPolicy: Never
          command: ["sh", "-c"]
          args:
            - |
              IDX="\${JOB_COMPLETION_INDEX:-0}"
              CHUNK="\$((IDX + 1))/${CHUNKS}"
              echo "=== chunk \${CHUNK} on \$(hostname) ==="
              # sigdfl (baked into the conformance image) resets the realtime
              # signals a CRI runtime leaves ignored on PID 1, so the trap-listing
              # fixtures don't see a spurious SIGRTMIN. No-op / absent elsewhere.
              RESET=""; command -v sigdfl >/dev/null 2>&1 && RESET="sigdfl"
              # "retry infra, keep results": a k8s Job's backoffLimit retries ANY
              # non-zero exit, but the harness exits non-zero when a FIXTURE fails.
              # A test failure is not an infrastructure failure — it must be a pod
              # SUCCESS whose pass/fail the aggregator reads from the log. So run
              # the suite, and exit 0 iff it produced a "Results:" line (it ran to
              # completion); exit 1 only when it could NOT run (missing tree, OOM,
              # node loss) so k8s reschedules that — and only that.
              out="\$(\$RESET ${SUITE_CMD} 2>&1)"; rc=\$?
              printf '%s\n' "\$out"
              if printf '%s\n' "\$out" | grep -q '^Results:'; then
                exit 0
              fi
              echo "INFRA: suite did not complete (rc=\$rc) — reschedulable" >&2
              exit 1
          resources:
            requests: { cpu: "${REQ_CPU}", memory: "${REQ_MEM}" }
            limits: { memory: "${LIM_MEM}" }
YAML
