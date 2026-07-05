#!/usr/bin/env bash
# run-bench.sh — build the bench image and produce the first bashy-vs-GNU
# fidelity + perf baselines inside it. One command, from the repo root or here.
#
# The bench image holds BOTH arms (GNU coreutils 9.11 + bash 5.3 built from
# source into /opt/gnu, and the linux bashy binary), so the numbers have zero
# cross-machine variance. See docs/coreutils-fidelity-perf-harness-spec.md.
#
# Prereqs (the script checks them):
#   - a podman: system `podman`, or the embedded `bashy podman` (set PODMAN="bashy podman").
#     NOTE embedded bashy podman needs its machine running (`bashy podman machine start`).
#   - the staged linux binaries next to this script:
#       perfbench-linux-<arch>  bashy-linux-<arch>
#     Build them (pure Go, CGO-free) with:
#       cd coreutils && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
#         go build -o eval/agent-shell/containers/perfbench-linux-"$ARCH" ./cmd/perfbench
#       cd bashy     && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
#         go build -o eval/agent-shell/containers/bashy-linux-"$ARCH" ./cmd/bashy
set -euo pipefail

cd "$(dirname "$0")"
ARCH="${ARCH:-arm64}"                       # arm64 | amd64
IMAGE="${IMAGE:-bashy-bench:9.11}"
PODMAN="${PODMAN:-podman}"                   # or: PODMAN="bashy podman"
RESULTS="${RESULTS:-../../../../results}"    # repo results/ (perf + fidelity baselines land here)

for b in "perfbench-linux-$ARCH" "bashy-linux-$ARCH"; do
  [ -f "$b" ] || { echo "run-bench: missing staged binary $b (see the header for the build command)" >&2; exit 2; }
done

echo "== building $IMAGE (GNU coreutils 9.11 + bash 5.3 from source; ~10-15 min first time) =="
$PODMAN build --build-arg TARGETARCH="$ARCH" -f bench.Containerfile -t "$IMAGE" .

echo "== recorded reference versions =="
$PODMAN run --rm --entrypoint cat "$IMAGE" /etc/gnu-coreutils-version /etc/gnu-bash-version /etc/bashy-version || true

mkdir -p "$RESULTS/perf/baseline" "$RESULTS/fidelity"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"   # host clock only labels the file, not the corpus (corpus is seeded)

RES_ABS="$(cd "$RESULTS" && pwd)"

echo "== inventory (present/missing vs the live registry) =="
$PODMAN run --rm "$IMAGE" list | tee "$RES_ABS/fidelity/inventory-$STAMP.txt"

echo "== corpus + fidelity + perf, chained in ONE container so the seeded corpus is shared =="
# Entrypoint is perfbench (one mode per call); use the in-image bashy as the shell
# to chain gen -> conformance -> run against a single /work corpus. /results is mounted out.
$PODMAN run --rm -v "$RES_ABS":/results --entrypoint /usr/local/bin/bashy "$IMAGE" -c '
  set -e
  perfbench gen
  echo "--- fidelity (byte-identical vs GNU coreutils 9.11) ---"
  perfbench conformance --format md   | tee /results/fidelity/conformance-'"$STAMP"'.md
  echo "--- perf A/B (four arms, three tiers) ---"
  perfbench run --format json         | tee /results/perf/baseline/'"$STAMP-$ARCH"'.json
'

echo "== done. baselines under $RES_ABS/{fidelity,perf} =="
