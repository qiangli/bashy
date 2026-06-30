#!/usr/bin/env bash
# posix-diff.sh — Phase 2 differential POSIX-conformance harness (in-container).
#
# Runs bashy AND the oracle shells in ONE container image so they share a
# byte-identical environment — same busybox coreutils, same $HOME. That
# isolates SHELL behavior: a host-vs-container comparison leaked non-shell
# noise (macOS BSD `wc -l` pads with spaces vs GNU/busybox `wc`; different
# $HOME values) that made bashy look divergent when it was not. Here bashy is
# cross-compiled to a Linux binary and mounted in; the oracles are native to
# the image, which is:
#
#   localhost/posix-shells = bash:5.3 (exact 5.3.x) + `apk add dash yash`
#
# so bash is the real 5.3 release, and dash/yash share its alpine/busybox
# userland. Each corpus script runs as a FILE ARG in a fresh cwd — hermetic,
# and it sidesteps the separate stdin-pipe heredoc bug (tracked elsewhere).
#
# Classification vs the oracle CONSENSUS:
#   MATCH      bashy agrees with every oracle.
#   DEVIATION  bashy disagrees where ALL oracles agree → high-confidence bug.
#   AMBIGUOUS  oracles disagree among themselves → bash extension / unspecified
#              behavior; annotated with which oracle(s) bashy matches.
# Plus a per-reference distance: how often bashy matches each oracle, now a
# fair same-environment number.
#
# Usage: scripts/posix-diff.sh [corpus-dir]   (run from the bashy repo root)
# Requires: a container runtime (docker or `bashy podman`) + Go.
# Exit 0 iff zero DEVIATIONs.

set -u
CORPUS=${1:-test/posix-corpus}
IMAGE=${POSIX_SHELLS_IMAGE:-localhost/posix-shells}

# The image's Containerfile and the oracle panel are both overridable, so the
# same harness drives a broader set of shells (see scripts/multishell-diff.sh).
# Defaults preserve the original Alpine bash-5.3 5-oracle panel exactly, so
# existing callers (oils-diff.sh) are unchanged.
SHELLS_DOCKERFILE=${SHELLS_DOCKERFILE:-$'FROM bash:5.3\nRUN apk add --no-cache dash yash zsh mksh\n'}
# ORACLE_SPEC: "name:cmd|name:cmd|…". The FIRST entry is the consensus anchor.
ORACLE_SPEC=${ORACLE_SPEC:-'bash53:bash --posix|dash:dash|yash:yash --posix|mksh:mksh|zsh:zsh --emulate sh'}

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif command -v bashy >/dev/null 2>&1; then OCI="bashy podman"
  fi
fi
[ -n "$OCI" ] || { echo "posix-diff: need a container runtime (docker / bashy podman)" >&2; exit 2; }
[ -d "$CORPUS" ] || { echo "posix-diff: corpus dir '$CORPUS' not found (run from repo root)" >&2; exit 2; }
CORPUS=$(cd "$CORPUS" && pwd)

# Ensure the combined oracle image (one shared userland for all oracles).
if ! $OCI image exists "$IMAGE" 2>/dev/null; then
  echo "posix-diff: building ${IMAGE} ..." >&2
  bd=$(mktemp -d)
  printf '%b' "$SHELLS_DOCKERFILE" > "$bd/Containerfile"
  $OCI build -q -t "$IMAGE" "$bd" >&2 || { echo "posix-diff: image build failed" >&2; exit 2; }
  rm -rf "$bd"
fi

# Cross-compile bashy to a Linux binary matching the container arch, so it runs
# natively inside the oracle image. Keep it under the repo (bin/) — macOS /tmp
# is a /private symlink that the container runtime refuses to bind-mount.
ARCH=$($OCI run --rm "$IMAGE" uname -m | tr -d '\r')
case "$ARCH" in
  aarch64|arm64) GOARCH=arm64 ;;
  x86_64|amd64)  GOARCH=amd64 ;;
  *) echo "posix-diff: unsupported container arch '$ARCH'" >&2; exit 2 ;;
esac
BIN="$PWD/bin/.bashy-linux-posixdiff-$$"
mkdir -p "$(dirname "$BIN")"
echo "posix-diff: building linux/$GOARCH bashy…" >&2
GOOS=linux GOARCH="$GOARCH" go build -o "$BIN" ./cmd/bash || { echo "posix-diff: bashy build failed" >&2; exit 2; }
trap 'rm -f "$BIN"' EXIT

# Run the whole comparison inside the image: bashy (mounted) + native oracles,
# every shell file-arg in a fresh cwd, identical environment.
$OCI run --rm -i -e ORACLE_SPEC="$ORACLE_SPEC" -v "$BIN:/bashy:ro" -v "$CORPUS:/corpus:ro" "$IMAGE" bash -s <<'INCONTAINER'
set -u
# Oracle panel, parsed from ORACLE_SPEC ("name:cmd|name:cmd|…"). All shells are
# native to the image so they share one userland (busybox coreutils, $HOME),
# isolating SHELL behavior. The first entry is the consensus anchor. Strict-POSIX
# shells (dash, ash, posh, yash) and the Korn/zsh family disambiguate "bash-only"
# vs portable behavior. csh/tcsh are deliberately absent — not POSIX shells.
ORACLES=""
declare -A CMD=()
_IFS_SAVE=$IFS; IFS='|'
for _spec in $ORACLE_SPEC; do
  _name=${_spec%%:*}; _cmd=${_spec#*:}
  ORACLES="$ORACLES $_name"; CMD[$_name]=$_cmd
done
IFS=$_IFS_SAVE
ORACLES=${ORACLES# }
REF=${ORACLES%% *}   # consensus anchor = first oracle

# runShell CMD  — copies $SCRIPT into a fresh cwd, runs CMD on it, echoes
# "ok|<output>" or "err|<output>" (stderr folded in; exact exit code ignored).
runShell() {
  local d out rc
  d=$(mktemp -d); cp "$SCRIPT" "$d/s"
  # Per-case timeout (busybox; all shells run in-container): a hanging case
  # (infinite loop, blocking read, etc.) must not stall the whole run. A
  # timeout shows as a non-zero exit, comparable across shells.
  out=$(cd "$d" && eval "timeout 10 $1 s" </dev/null 2>&1); rc=$?
  rm -rf "$d"
  printf '%s|%s' "$([ "$rc" -eq 0 ] && echo ok || echo err)" "$out"
}

declare -A REF_MATCH REF_TOTAL
match=0 dev=0 amb=0
for SCRIPT in /corpus/*.sh; do
  base=$(basename "$SCRIPT")
  by=$(runShell "/bashy --posix")
  declare -A okey=()
  for n in $ORACLES; do okey[$n]=$(runShell "${CMD[$n]}"); done

  agree=1; first="${okey[$REF]}"
  for n in $ORACLES; do [ "${okey[$n]}" = "$first" ] || agree=0; done
  for n in $ORACLES; do
    REF_TOTAL[$n]=$(( ${REF_TOTAL[$n]:-0} + 1 ))
    [ "$by" = "${okey[$n]}" ] && REF_MATCH[$n]=$(( ${REF_MATCH[$n]:-0} + 1 ))
  done

  if [ "$agree" -eq 0 ]; then
    amb=$((amb+1)); mm=""
    for n in $ORACLES; do [ "$by" = "${okey[$n]}" ] && mm="$mm $n"; done
    [ -z "$mm" ] && mm=" none"
    echo "AMBIG  $base (oracles disagree; bashy matches:$mm)"
  elif [ "$by" = "$first" ]; then
    match=$((match+1)); echo "MATCH  $base"
  else
    dev=$((dev+1)); echo "DEVIATION  $base"
    echo "   bashy : [$by]"
    echo "   oracle: [$first]  ($ORACLES)"
  fi
done

echo "=== $match match / $dev deviation / $amb ambiguous ($((match+dev+amb)) scripts) ==="
echo "--- per-reference distance (bashy agrees with each, SAME environment) ---"
for n in $ORACLES; do
  m=${REF_MATCH[$n]:-0}; t=${REF_TOTAL[$n]:-0}; p="n/a"; [ "$t" -gt 0 ] && p="$(( m*100/t ))%"
  printf "  bashy vs %-7s : %d/%d (%s)\n" "$n" "$m" "$t" "$p"
done
echo "  (bash drop-in fidelity anchor: 86/86 on bash's own 5.3 fixture suite — see make test-bash)"
[ "$dev" -eq 0 ]
INCONTAINER
