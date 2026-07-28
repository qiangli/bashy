#!/usr/bin/env bash
# Emit one native-platform DKS Job. The selected vk-native node executes the
# installed bashy directly on its registered host OS; no Linux container or
# agent-node result can satisfy this proof.
set -euo pipefail

NAME="${NAME:-bashy-native-smoke}"
NS="${NS:-default}"
TARGET_OS="${TARGET_OS:?set TARGET_OS to linux, darwin, or windows}"
TARGET_ARCH="${TARGET_ARCH:-}"
TARGET_HOST="${TARGET_HOST:-}"
ARTIFACT_URL="${ARTIFACT_URL:?set ARTIFACT_URL to a published bashy release archive}"
ARTIFACT_SHA256="${ARTIFACT_SHA256:?set ARTIFACT_SHA256 to the archive checksum}"
ARTIFACT_PATH="${ARTIFACT_PATH:-bashy}"
TTL="${TTL:-3600}"
BACKOFF="${BACKOFF:-1}"

case "$TARGET_OS" in linux|darwin|windows) ;; *)
  echo "dks-native-smoke-job: TARGET_OS must be linux, darwin, or windows" >&2
  exit 2
esac

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
              os=\$(uname -s | tr A-Z a-z)
              arch=\$(uname -m)
              version=\$(bashy --version | head -1)
              [ "\$(bashy -c 'echo runtime-ok')" = runtime-ok ]
              bashy curl --version >/dev/null
              printf 'DKS_RESULT:{"schema":1,"classification":"pass","lane":"native-platform","os":"%s","arch":"%s","version":"%s"}\n' "\$os" "\$arch" "\$version"
YAML
