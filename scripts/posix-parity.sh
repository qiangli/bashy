#!/usr/bin/env bash
# posix-parity.sh — Phase 1 POSIX-mode parity probe.
# Compares `bashy --posix` against real GNU bash 5.3 `--posix` (via
# `docker run bash:5.3`) on a corpus of probes, one per testable behavior from
# docs/posix-mode-behaviors.md. Output normalizes the program-name prefix
# (bashy:/bash: -> SH:) and line numbers so only semantic differences show.
#
# Usage: scripts/posix-parity.sh   (needs bin/bashy built + docker)
set -u
BASHY=${BASHY:-./bin/bashy}
# Normalize the program-name prefix (any path ending in bashy/bash -> SH) and
# line numbers, so only semantic differences remain.
norm() { sed -E 's#[^ ]*(bashy|bash):#SH:#g; s/line [0-9]+/line N/g'; }

# probe NUM 'script'
declare -a NUMS SCRIPTS
add() { NUMS+=("$1"); SCRIPTS+=("$2"); }

add 1  'echo "PC=${POSIXLY_CORRECT-unset}"'
add 12 'eval() { :; }; echo after'
add 14 'set -- a b; n=#; echo "${!n}"; echo after'
add 17 'false; $(true); echo "dollar?=$?"'
add 19 'export() { echo FUNC; }; export X=1; echo "x=$X"'
add 33 'echo $((1 +)); echo after'
add 34 'echo ${undefinedvar?boom}; echo after'
add 35 'set -o nosuchoption; echo after'
add 36 'readonly rr=1; rr=2; echo after'
add 38 'readonly ii=1; for ii in a b; do :; done; echo after'
add 39 '. /no/such/file/xyz; echo after'
add 40 'eval "if"; echo after'
add 41 'unset 1badname; echo after'
add 44 'set -e; v=$(false; echo inner); echo "out v=$v rc=$?"'
add 45 'set -- a; shift 5; echo "rc=$?"'
add 48 'alias zz=yy; alias'
add 53 'export EE=1; export | grep EE'
add 58 'kill -l'
add 59 'kill -SIGTERM 2>&1 | head -1; echo "rc=$?"'
add 64 'ff() { :; }; gg=1; set | grep -E "^ff" | head -1; echo "---"'
add 65 'xx=hello; set | grep "^xx="'
add 68 'trap "echo x" INT; trap -p INT'
add 73 'readonly rv=1; unset -v rv; echo after'

# --- run bashy locally (via -c; bashy doesn't accept -s yet) ---
declare -a BY
for i in "${!NUMS[@]}"; do
  BY[$i]=$("$BASHY" --posix -c "${SCRIPTS[$i]}" 2>&1 | norm | tr '\n' '~')
done
# --- run bash 5.3 in one docker container, marker-delimited per probe ---
PROBES=$(for i in "${!NUMS[@]}"; do printf '%s\t%s\n' "$i" "${SCRIPTS[$i]}"; done)
RAW=$(printf '%s\n' "$PROBES" | docker run --rm -i -e HOME=/tmp bash:5.3 bash -c '
  tab=$(printf "\t")
  while IFS="$tab" read -r idx script; do
    echo "@@@PROBE:$idx@@@"
    bash --posix -c "$script" 2>&1
  done')
declare -a BH
cur=""
while IFS= read -r line; do
  case "$line" in
    @@@PROBE:*@@@) cur=${line#@@@PROBE:}; cur=${cur%@@@} ;;
    *) [ -n "$cur" ] && BH[$cur]="${BH[$cur]:-}$line"$'\n' ;;
  esac
done < <(printf '%s\n' "$RAW")
for i in "${!NUMS[@]}"; do
  BH[$i]=$(printf '%s' "${BH[$i]:-}" | norm | tr '\n' '~')
done

# --- compare ---
match=0; diff=0
for i in "${!NUMS[@]}"; do
  if [ "${BY[$i]}" = "${BH[$i]:-<<no bash>>}" ]; then
    echo "MATCH  #${NUMS[$i]}"; match=$((match+1))
  else
    echo "DIFF   #${NUMS[$i]}"
    echo "   bashy: [${BY[$i]}]"
    echo "   bash:  [${BH[$i]:-<<no bash>>}]"
    diff=$((diff+1))
  fi
done
echo "=== $match match / $diff diff / $((match+diff)) probed ==="
