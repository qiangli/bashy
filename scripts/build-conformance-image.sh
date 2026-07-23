#!/usr/bin/env bash
# Build a SELF-CONTAINED conformance image for k8s/DKS (Argo or the native Job).
# Bakes the Linux testee + harness + fixture tree + chunk manifest into an image
# that needs no host mounts. See tools/bash53-container/Containerfile.k8s* and
# scripts/dag-to-k8s-job.sh (the tier-5 "B path" executor).
#
#   SUITE=bash53 (default) — bash-5.3 fixtures via tools/bash53suite + chunks.json
#   SUITE=yash             — yash POSIX (-p) fixtures via tools/yashsuite +
#                            yash-chunks.json; corpus cloned from .yash-tests/
#                            (GPL, gitignored — never committed, baked into a
#                            throwaway runtime image only).
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"; cd "$REPO"
SUITE="${SUITE:-bash53}"
ARCH="${ARCH:-amd64}"                                  # cluster node arch
OCI="${OCI:-$REPO/bin/bashy podman}"
# $OS=Windows_NT is set only on Windows. On the Windows DKS node the podman
# CLIENT panics tarring a large build context (docs/todo 9d2b4e7a0c15); we route
# build+run through the podman machine, which reads the /mnt/<drive> WSL mount.
on_windows() { [ "${OS:-}" = "Windows_NT" ]; }
# C:/foo | C:\foo | /c/foo  ->  /mnt/c/foo  (WSL mount of a host path)
to_vm_path() {
  local p="${1//\\//}"
  case "$p" in
    [A-Za-z]:/*) printf '/mnt/%s%s' "$(printf '%s' "${p%%:*}" | tr 'A-Z' 'a-z')" "${p#*:}" ;;
    /[A-Za-z]/*) printf '/mnt%s' "$p" ;;
    *)           printf '%s' "$p" ;;
  esac
}
# The podman machine to build inside — prefer the running one, else the first.
# `podman machine ssh` needs an explicit name unless a machine is the default;
# bashy's machine ("bashy") is not marked default, so we always name it.
vm_machine() {
  local m
  m="$($OCI machine list --format '{{.Name}} {{.Running}}' 2>/dev/null | awk '$2=="true"{print $1; exit}')"
  [ -n "$m" ] || m="$($OCI machine list --format '{{.Name}}' 2>/dev/null | head -1)"
  printf '%s' "$m"
}
# coreutils mktemp reads TMPDIR; on Windows it is often unset while $TEMP holds
# the native temp dir — point mktemp at it so `mktemp -d` below succeeds.
on_windows && [ -z "${TMPDIR:-}" ] && [ -n "${TEMP:-}" ] && export TMPDIR="$TEMP"
CTX="$(mktemp -d)/ctx"; mkdir -p "$CTX/bin"
trap 'rm -rf "$(dirname "$CTX")"' EXIT
log(){ printf '>> %s\n' "$*" >&2; }

case "$SUITE" in
bash53)
  IMAGE="${IMAGE:-localhost/bash53-conformance-k8s:latest}"
  mkdir -p "$CTX/bin/bash-linux-$ARCH" "$CTX/bash53"
  log "building linux/$ARCH testee + bash53 harness…"
  GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build -trimpath -o "$CTX/bin/bash-linux-$ARCH/bash" ./cmd/bash
  GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build -trimpath -o "$CTX/bin/bash53suite-linux-$ARCH" ./tools/bash53suite
  log "staging fixture tree (strip prebuilt helpers; keep support/*.c)…"
  # symlink on unix; a real dir on a Windows checkout (symlinks not preserved).
  FX="$(readlink external/bash-5.3 2>/dev/null || echo external/bash-5.3)"
  cp -R "$FX/." "$CTX/bash53/"
  rm -f "$CTX/bash53/tests/recho" "$CTX/bash53/tests/zecho" "$CTX/bash53/tests/xcase" "$CTX/bash53/tests/printenv"
  cp chunks.json "$CTX/chunks.json"
  cp tools/bash53-container/Containerfile.k8s "$CTX/Containerfile"
  sed -i.bak "s/^ENV BASH53_ARCH=.*/ENV BASH53_ARCH=$ARCH/" "$CTX/Containerfile" && rm -f "$CTX/Containerfile.bak"
  SELFCHECK_ENV="-e CHUNK=3/8 -e BASH53_ARCH=$ARCH"
  ;;
yash)
  IMAGE="${IMAGE:-localhost/yash-conformance-k8s:latest}"
  YT="$REPO/.yash-tests"
  [ -d "$YT/tests" ] || { echo "build-conformance-image: yash corpus missing — clone into $YT first (git clone --depth 1 https://github.com/magicant/yash.git $YT)" >&2; exit 2; }
  log "building linux/$ARCH testee + yashsuite runner…"
  mkdir -p "$CTX/bin/bash-linux-$ARCH"
  GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build -trimpath -o "$CTX/bin/bash-linux-$ARCH/bash" ./cmd/bash
  GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build -trimpath -o "$CTX/bin/yashsuite-linux-$ARCH" ./tools/yashsuite
  log "staging yash tests/ and pruning to the manifest's shell-only -p fixtures…"
  cp -R "$YT/tests" "$CTX/yashtests"
  # Keep exactly the -p fixtures the manifest lists; drop the rest so
  # yashsuite's discovery bijects with yash-chunks.json (it validates strictly).
  keep="$(python3 -c 'import json,sys; m=json.load(open("yash-chunks.json")); print("\n".join(f["name"] for c in m["chunks"] for f in c["fixtures"]))')"
  ( cd "$CTX/yashtests"
    for t in *-p.tst; do
      b="${t%.tst}"
      printf '%s\n' "$keep" | grep -qxF "$b" || rm -f "$t"
    done )
  kept=$(ls "$CTX/yashtests"/*-p.tst | wc -l | tr -d ' ')
  log "baked $kept shell-only -p fixtures (manifest chunk_count=$(go run ./tools/yashsuite --chunks-manifest yash-chunks.json --chunk-count))"
  cp yash-chunks.json "$CTX/yash-chunks.json"
  cp tools/bash53-container/Containerfile.k8s.yash "$CTX/Containerfile"
  sed -i.bak "s/^ENV YASH_ARCH=.*/ENV YASH_ARCH=$ARCH/" "$CTX/Containerfile" && rm -f "$CTX/Containerfile.bak"
  SELFCHECK_ENV="-e CHUNK=1/8 -e YASH_ARCH=$ARCH"
  ;;
*)
  echo "build-conformance-image: unknown SUITE=$SUITE (want bash53|yash)" >&2; exit 2 ;;
esac

log "building image $IMAGE (linux/$ARCH, SUITE=$SUITE)…"
# SQUASH=1 collapses ALL layers into one so the image carries no inter-layer
# whiteout (.wh.*) markers. Nested container runtimes (k3s-agent inside podman
# inside a VM) cannot create the device-node whiteouts overlayfs uses, so a
# multi-layer image fails `ctr images import` with "convert whiteout … operation
# not permitted". A single flat layer imports cleanly on any nesting depth.
SQUASH_FLAG=""
[ "${SQUASH:-0}" = "1" ] && SQUASH_FLAG="--squash-all"

# On WINDOWS the podman CLIENT panics tarring a large native build context
# (docs/todo 9d2b4e7a0c15); build + self-check INSIDE the podman machine, which
# reads the context off its /mnt/<drive> WSL mount — same host, same containerd
# the node uses, so it stays a native build.
if on_windows; then
  VMM="$(vm_machine)"; [ -n "$VMM" ] || { echo "build-conformance-image: no podman machine found (run: $OCI machine init)" >&2; exit 2; }
  vmctx="$(to_vm_path "$CTX")"
  $OCI machine ssh "$VMM" "cd '$vmctx' && podman build $SQUASH_FLAG --platform linux/$ARCH -t '$IMAGE' -f Containerfile ." 2>&1 | grep -iE 'STEP|COMMIT|Successfully|error' | tail -4
else
  $OCI build $SQUASH_FLAG --platform "linux/$ARCH" -t "$IMAGE" -f "$CTX/Containerfile" "$CTX" 2>&1 | grep -iE 'STEP|COMMIT|Successfully|error' | tail -4
fi

log "done. self-check: run one chunk inside the image (no mounts)…"
if on_windows; then
  $OCI machine ssh "$VMM" "podman run --rm $SELFCHECK_ENV '$IMAGE'" 2>&1 | grep -iE 'Results:|FAIL|error' | tail -2
else
  $OCI run --rm --platform "linux/$ARCH" $SELFCHECK_ENV "$IMAGE" 2>&1 | grep -iE 'Results:|FAIL|error' | tail -2
fi
echo "IMAGE=$IMAGE  ARCH=$ARCH  SUITE=$SUITE" >&2
