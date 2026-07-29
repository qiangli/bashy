#!/usr/bin/env bash
# Emit one native-platform DKS Job. vk-native materializes a checksum-verified
# Bashy release archive and executes it directly on the registered host OS.
set -euo pipefail

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
EXECUTOR_NODE="${EXECUTOR_NODE:?set EXECUTOR_NODE to the scheduled vk-native node name}"
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

selector_extra=""
[ -n "$TARGET_ARCH" ] && selector_extra="${selector_extra}
        kubernetes.io/arch: ${TARGET_ARCH}"
[ -n "$TARGET_HOST" ] && selector_extra="${selector_extra}
        outpost.dhnt.io/host: ${TARGET_HOST}"

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
          imagePullPolicy: Never
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
