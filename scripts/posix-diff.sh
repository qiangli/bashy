#!/usr/bin/env bash
# posix-diff.sh — Phase 2 differential POSIX-conformance harness.
#
# Runs every script in a corpus through `bashy --posix` and one or more
# independent near-POSIX oracle shells (dash, bash 5.3 --posix, …), comparing
# observable behavior — stdout (normalized) + success/fail (exit 0 vs not),
# deliberately ignoring diagnostic wording and exact exit-code value (POSIX
# mandates neither). Each script is fed on STDIN so $0/arg handling is uniform.
#
# Classification per script:
#   MATCH      bashy agrees with EVERY oracle.
#   DEVIATION  bashy disagrees with ALL oracles that agree among themselves
#              → high-confidence bashy bug (triage + fix in sh).
#   AMBIGUOUS  the oracles disagree with each other → not a clear bashy gap
#              (shell-specific extension / unspecified behavior); reported, not
#              counted as a bashy deviation.
#
# Oracles auto-detect: local `dash` if present, and `bash 5.3 --posix` via the
# container runtime (docker or `ycode podman`). yash slots in here once built
# (add a YASH= entry). Override the corpus dir as $1 (default test/posix-corpus).
#
# Usage: scripts/posix-diff.sh [corpus-dir]
# Exit:  0 iff zero DEVIATIONs.

BASHY=${BASHY:-./bin/bash}
CORPUS=${1:-test/posix-corpus}
# Absolutize both — the run loop cd's each shell into a fresh temp dir, so the
# script path (fed on stdin) and the binary must not be cwd-relative.
[ -d "$CORPUS" ] && CORPUS=$(cd "$CORPUS" && pwd)
case "$BASHY" in /*) ;; *) [ -e "$BASHY" ] && BASHY=$(cd "$(dirname "$BASHY")" && pwd)/$(basename "$BASHY") ;; esac

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif command -v ycode >/dev/null 2>&1; then OCI="ycode podman"
  fi
fi

# Build the oracle list (each reads the script on stdin; stderr discarded).
# yash --posix is the strictest near-POSIX shell and the best tiebreaker; it is
# used when the localhost/yash-oracle image is present (build once:
# `printf 'FROM alpine\nRUN apk add --no-cache yash\n' | $OCI build -t localhost/yash-oracle -f - .`).
declare -a ORACLE_NAMES=()
command -v dash >/dev/null 2>&1 && ORACLE_NAMES+=("dash")
[ -n "$OCI" ] && ORACLE_NAMES+=("bash53")
if [ -n "$OCI" ] && $OCI image exists localhost/yash-oracle 2>/dev/null; then
  ORACLE_NAMES+=("yash")
fi

if [ ${#ORACLE_NAMES[@]} -eq 0 ]; then
  echo "posix-diff: no oracle shells available (need dash and/or a container runtime)" >&2
  exit 2
fi
if [ ! -x "$BASHY" ]; then
  echo "posix-diff: $BASHY not built — run 'make build' first" >&2
  exit 2
fi

match=0 deviation=0 ambiguous=0
declare -a DEV_LIST AMB_LIST
# Per-reference conformance distance: how often bashy agrees with each oracle
# shell, independent of consensus. Answers "how close is bashy to bash / dash /
# yash?" — a behavioral-distance metric per reference.
declare -A REF_MATCH REF_TOTAL

shopt -s nullglob
for f in "$CORPUS"/*.sh; do
  # Run each shell in a FRESH, writable working directory so file-creating /
  # globbing scripts are hermetic and behave identically across shells (local
  # shells cd into a per-script temp dir; each container run is already a fresh
  # container — give it a writable cwd via -w /tmp). Scripts should use cwd-
  # relative paths, not absolute /tmp paths, to stay isolated.
  tmpd=$(mktemp -d)
  byout=$(cd "$tmpd" && "$BASHY" --posix <"$f" 2>/dev/null); byrc=$?
  bykey="$([ $byrc -eq 0 ] && echo ok || echo err)|$byout"

  # oracles
  declare -a okeys=()
  for name in "${ORACLE_NAMES[@]}"; do
    od=$(mktemp -d)
    case "$name" in
      dash)   oout=$(cd "$od" && dash <"$f" 2>/dev/null); orc=$? ;;
      bash53) oout=$($OCI run --rm -i -w /tmp bash:5.3 bash --posix <"$f" 2>/dev/null); orc=$? ;;
      yash)   oout=$($OCI run --rm -i -w /tmp localhost/yash-oracle yash --posix <"$f" 2>/dev/null); orc=$? ;;
    esac
    rm -rf "$od"
    okey="$([ $orc -eq 0 ] && echo ok || echo err)|$oout"
    okeys+=("$okey")
    REF_TOTAL[$name]=$(( ${REF_TOTAL[$name]:-0} + 1 ))
    [ "$bykey" = "$okey" ] && REF_MATCH[$name]=$(( ${REF_MATCH[$name]:-0} + 1 ))
  done
  rm -rf "$tmpd"

  # do the oracles agree among themselves?
  oracles_agree=1
  first="${okeys[0]}"
  for k in "${okeys[@]}"; do [ "$k" = "$first" ] || oracles_agree=0; done

  base=$(basename "$f")
  if [ "$oracles_agree" -eq 0 ]; then
    ambiguous=$((ambiguous+1)); AMB_LIST+=("$base")
    # Which oracle(s) does bashy match? Distinguishes "bashy is faithfully
    # bash-like on a bash extension" (matches bash53) from "bashy is its own
    # thing" (matches none → worth a look even though it's not a hard deviation).
    mm=""
    for i in "${!ORACLE_NAMES[@]}"; do
      [ "$bykey" = "${okeys[$i]}" ] && mm="$mm ${ORACLE_NAMES[$i]}"
    done
    [ -z "$mm" ] && mm=" none"
    echo "AMBIG  $base (oracles disagree; bashy matches:$mm)"
  elif [ "$bykey" = "$first" ]; then
    match=$((match+1))
    echo "MATCH  $base"
  else
    deviation=$((deviation+1)); DEV_LIST+=("$base")
    echo "DEVIATION  $base"
    echo "   bashy : [${bykey}]"
    echo "   oracle: [${first}]  (${ORACLE_NAMES[*]})"
  fi
done

echo "=== $match match / $deviation deviation / $ambiguous ambiguous ($((match+deviation+ambiguous)) scripts) ==="
echo "--- per-reference behavioral distance (bashy agrees with each shell) ---"
for name in "${ORACLE_NAMES[@]}"; do
  m=${REF_MATCH[$name]:-0}; t=${REF_TOTAL[$name]:-0}
  pct="n/a"; [ "$t" -gt 0 ] && pct="$(( m * 100 / t ))%"
  printf "  bashy vs %-7s : %d/%d (%s)\n" "$name" "$m" "$t" "$pct"
done
echo "  (bash drop-in fidelity anchor: 86/86 on bash's own 5.3 fixture suite — see make test-bash)"
[ "$deviation" -eq 0 ]
