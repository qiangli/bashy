#!/usr/bin/env bash
# Glue only: mirror `tutor status N --line` into a Klatch group header and post
# a milestone when the current step changes. tutor and klatch never see each other.
#   chat-status.sh SPRINT CONFIG.json GROUP [--every SECONDS] [--once]
set -euo pipefail
usage='Usage: chat-status.sh SPRINT CONFIG.json GROUP [--every SECONDS] [--once]'
(( $# >= 3 )) || { echo "$usage" >&2; exit 2; }
sprint=$1 config=$2 group=$3
shift 3
every=30 once=false
while (( $# )); do
  case $1 in
    --every) every=${2:?$usage}; shift 2 ;;
    --once) once=true; shift ;;
    *) echo "$usage" >&2; exit 2 ;;
  esac
done
bashy=${BASHY_BIN:-bashy}
last_line= last_step=
while :; do
  if line=$("$bashy" tutor status "$sprint" --line 2>/dev/null) && [[ -n $line ]]; then
    if [[ $line != "$last_line" ]]; then
      printf '%s\n' "$line" | "$bashy" klatch topic "$config" "$group" - >/dev/null || true
      last_line=$line
    fi
    step=
    if [[ $line =~ [Ss]tep[-\ ]?([0-9]+) ]]; then step=${BASH_REMATCH[1]}; fi
    if [[ -n $step && $step != "$last_step" ]]; then
      if [[ -n $last_step ]]; then
        printf 'Now: %s\n' "$line" | "$bashy" klatch post "$config" "$group" - >/dev/null || true
      fi
      last_step=$step
    fi
  fi
  [[ $once == true ]] && break
  sleep "$every"
done
