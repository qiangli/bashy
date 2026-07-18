#!/usr/bin/env bash
# vsc-profile.sh - public-safe VSC-PCTS execution profile guard.
#
# This script validates the execution shape before any licensed harness dispatch.
# It does not read the licensed suite and must not print host identifiers,
# credentials, suite source, journals, or raw private logs.
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: scripts/vsc-profile.sh validate --profile cert|reference|campaign [options]

options:
  --workers N          worker count (default: 1)
  --chunks N           chunk count (default: 1)
  --repeat N           repeat count (default: 2 for cert/reference, 1 for campaign)
  --cache on|off       cache reuse (default: off)
  --retries N          retry count (default: 0)
  --shard N            shard index (forbidden for cert/reference)
  --of N               shard denominator (forbidden for cert/reference)
  --sut-command NAME   SUT command name (cert/reference require sh; default: sh)
  --atom-file PATH     optional public atom fixture to emit in declared order
EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 2
}

is_uint() {
  case "${1:-}" in
    ''|*[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

require_uint() {
  local name=$1 value=$2
  is_uint "$value" || die "$name must be an unsigned integer, got: $value"
}

require_on_off() {
  case "$2" in
    on|off) ;;
    *) die "$1 must be on or off, got: $2" ;;
  esac
}

profile=cert
workers=1
chunks=1
repeat=''
cache=off
retries=0
shard=''
of=''
sut_command=sh
atom_file=''

cmd=${1:-}
[ -n "$cmd" ] || { usage; exit 2; }
shift
[ "$cmd" = validate ] || die "unknown command: $cmd"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --profile) shift; profile=${1:-};;
    --workers) shift; workers=${1:-};;
    --chunks) shift; chunks=${1:-};;
    --repeat) shift; repeat=${1:-};;
    --cache) shift; cache=${1:-};;
    --retries) shift; retries=${1:-};;
    --shard) shift; shard=${1:-};;
    --of) shift; of=${1:-};;
    --sut-command) shift; sut_command=${1:-};;
    --atom-file) shift; atom_file=${1:-};;
    -h|--help) usage; exit 0;;
    *) die "unknown option: $1";;
  esac
  [ "$#" -gt 0 ] || break
  shift
done

case "$profile" in
  cert|reference|campaign) ;;
  *) die "profile must be cert, reference, or campaign, got: $profile" ;;
esac

[ -n "$repeat" ] || {
  case "$profile" in
    campaign) repeat=1 ;;
    *) repeat=2 ;;
  esac
}

require_uint workers "$workers"
require_uint chunks "$chunks"
require_uint repeat "$repeat"
require_uint retries "$retries"
require_on_off cache "$cache"
[ -z "$shard" ] || require_uint shard "$shard"
[ -z "$of" ] || require_uint of "$of"
[ -z "$atom_file" ] || [ -f "$atom_file" ] || die "atom file not found: $atom_file"

if [ "$profile" = cert ]; then
  [ "$workers" -eq 1 ] || die "cert profile requires workers=1 before dispatch"
  [ "$chunks" -eq 1 ] || die "cert profile rejects chunking before dispatch"
  [ "$cache" = off ] || die "cert profile rejects cache reuse before dispatch"
  [ "$retries" -eq 0 ] || die "cert profile rejects retries before dispatch"
  [ "$repeat" -ge 2 ] || die "cert profile requires repeat>=2 before dispatch"
  [ -z "$shard" ] || die "cert profile rejects shard flags before dispatch"
  [ -z "$of" ] || die "cert profile rejects shard flags before dispatch"
fi

if [ "$profile" = reference ]; then
  [ "$workers" -eq 1 ] || die "reference profile requires workers=1"
  [ "$chunks" -eq 1 ] || die "reference profile requires chunks=1"
  [ -z "$shard" ] || die "reference profile rejects shard flags"
  [ -z "$of" ] || die "reference profile rejects shard flags"
fi

if [ "$profile" = cert ] || [ "$profile" = reference ]; then
  [ "$sut_command" = sh ] || die "$profile profile requires the SUT to resolve as sh on PATH"
fi

sut_path=$(PATH=${PATH:-} command -v "$sut_command" 2>/dev/null || true)
[ -n "$sut_path" ] || die "SUT command not found on PATH: $sut_command"

if [ "$profile" = campaign ]; then
  certifiable=false
  notice='NOT CERTIFIABLE: campaign profile is development feedback only.'
else
  certifiable=true
  notice='certification-shaped profile'
fi

start_context=$(uname -srm 2>/dev/null | tr '\n' ' ')
end_context=$start_context

printf 'vsc_profile=%s\n' "$profile"
printf 'certifiable=%s\n' "$certifiable"
printf 'notice=%s\n' "$notice"
printf 'workers=%s\n' "$workers"
printf 'chunks=%s\n' "$chunks"
printf 'repeat=%s\n' "$repeat"
printf 'cache=%s\n' "$cache"
printf 'retries=%s\n' "$retries"
printf 'sut_command=%s\n' "$sut_command"
printf 'sut_path=%s\n' "$sut_path"
printf 'context_start=%s\n' "$start_context"
printf 'context_end=%s\n' "$end_context"

if [ -n "$atom_file" ]; then
  printf 'atoms_begin\n'
  sed '/^[[:space:]]*$/d' "$atom_file"
  printf 'atoms_end\n'
fi
