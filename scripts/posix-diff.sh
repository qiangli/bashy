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

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif command -v ycode >/dev/null 2>&1; then OCI="ycode podman"
  fi
fi

# Build the oracle list (each reads the script on stdin; stderr discarded).
declare -a ORACLE_NAMES=()
command -v dash >/dev/null 2>&1 && ORACLE_NAMES+=("dash")
[ -n "$OCI" ] && ORACLE_NAMES+=("bash53")

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

shopt -s nullglob
for f in "$CORPUS"/*.sh; do
  byout=$("$BASHY" --posix <"$f" 2>/dev/null); byrc=$?
  bykey="$([ $byrc -eq 0 ] && echo ok || echo err)|$byout"

  # oracles
  declare -a okeys=()
  for name in "${ORACLE_NAMES[@]}"; do
    case "$name" in
      dash)   oout=$(dash <"$f" 2>/dev/null); orc=$? ;;
      bash53) oout=$($OCI run --rm -i bash:5.3 bash --posix <"$f" 2>/dev/null); orc=$? ;;
    esac
    okeys+=("$([ $orc -eq 0 ] && echo ok || echo err)|$oout")
  done

  # do the oracles agree among themselves?
  oracles_agree=1
  first="${okeys[0]}"
  for k in "${okeys[@]}"; do [ "$k" = "$first" ] || oracles_agree=0; done

  base=$(basename "$f")
  if [ "$oracles_agree" -eq 0 ]; then
    ambiguous=$((ambiguous+1)); AMB_LIST+=("$base")
    echo "AMBIG  $base (oracles disagree)"
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
[ "$deviation" -eq 0 ]
