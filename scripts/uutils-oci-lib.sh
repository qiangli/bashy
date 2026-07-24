#!/usr/bin/env bash
# Shared, sourceable construction helpers for the contained uutils runner.

resolve_uutils_oci() {
  if [ -n "${UUTILS_OCI:-}" ]; then
    UUTILS_OCI_CMD=("$UUTILS_OCI")
  elif command -v docker >/dev/null 2>&1; then
    UUTILS_OCI_CMD=(docker)
  elif command -v bashy >/dev/null 2>&1; then
    UUTILS_OCI_CMD=(bashy podman)
  else
    echo "need docker or bashy podman (or set UUTILS_OCI to an OCI-compatible executable)" >&2
    return 2
  fi
}

build_uutils_oci_args() {
  local image=$1 user=$2 memory=$3 pids=$4 cidfile=$5
  local name=$6 archive=$7 sut=$8 inner=$9 out=${10} threads=${11}
  UUTILS_OCI_ARGS=(
    run --rm --pull=never
    --cidfile "$cidfile"
    --name "$name"
    --network=none
    --memory "$memory"
    --memory-swap "$memory"
    --pids-limit "$pids"
    --user "$user"
    --read-only
    --cap-drop=ALL
    --security-opt=no-new-privileges
    --tmpfs /work:rw,nosuid,nodev,size=8g,mode=1777
    --tmpfs /tmp:rw,nosuid,nodev,size=256m,mode=1777
    --mount "type=bind,src=$archive,dst=/input/uutils.tar,readonly"
    --mount "type=bind,src=$sut,dst=/input/coreutils,readonly"
    --mount "type=bind,src=$inner,dst=/input/run.sh,readonly"
    --mount "type=bind,src=$out,dst=/out"
    --env "THREADS=$threads"
    "$image"
    /bin/bash /input/run.sh
  )
}
