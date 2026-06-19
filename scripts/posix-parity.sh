#!/usr/bin/env bash
# posix-parity.sh — Phase 1 POSIX-mode conformance probe.
#
# Asserts that `bashy --posix` is POSIX-mode-equivalent to GNU bash 5.3
# `--posix` (reference via `docker run bash:5.3`, since macOS ships bash 3.2)
# on the mechanically-testable behaviors from docs/posix-mode-behaviors.md.
#
# This measures POSIX CONFORMANCE, which is SEMANTIC — not byte-exact bash
# mimicry. For each probe we compare:
#   (a) stdout (the observable program output), normalized; and
#   (b) whether the shell SUCCEEDED or FAILED (exit 0 vs non-zero).
# We deliberately do NOT compare the exact diagnostic WORDING or the specific
# exit-CODE value — POSIX mandates neither (it requires that a non-interactive
# shell *exit* on certain errors, not which message or code). The shell's own
# stderr is discarded; a probe that needs to observe an error redirects it
# into stdout itself (e.g. `cmd 2>&1 | head -1`).
#
# Probes whose result is inherently host/OS-specific (e.g. the `kill -l`
# signal set differs Darwin vs Linux) are marked INFO and excluded from the
# pass/fail count — only the POSIX-relevant aspect (format) is asserted.
#
# Usage: scripts/posix-parity.sh   (needs bin/bashy built + docker)
# Exit: 0 iff every non-INFO probe matches.
set -u
BASHY=${BASHY:-./bin/bashy}

# Run bashy in a clean, minimal environment so host variables (API keys, etc.)
# don't leak into its `export`/`set` listings while the docker bash sees a
# pristine env — that asymmetry is a harness artifact, not a real difference.
CLEAN=(env -i HOME=/tmp PATH=/usr/bin:/bin)

# Normalize the program-name prefix (any path ending in bashy/bash -> SH) and
# line numbers, so only semantic differences in stdout remain.
norm() { sed -E 's#[^ ]*(bashy|bash):#SH:#g; s/line [0-9]+/line N/g'; }

# add NUM 'script'         — a strict conformance probe (counted)
# info NUM 'script' 'note' — host/OS-specific; reported but not counted
declare -a NUMS SCRIPTS NOTE
add()  { NUMS+=("$1"); SCRIPTS+=("$2"); NOTE+=(""); }
info() { NUMS+=("$1"); SCRIPTS+=("$2"); NOTE+=("$3"); }

add  1  'echo "PC=${POSIXLY_CORRECT-unset}"'
add  12 'eval() { :; }; echo after'
add  14 'set -- a b; n=#; echo "${!n}"; echo after'
add  17 'false; $(true); echo "dollar?=$?"'
add  19 'export() { echo FUNC; }; export X=1; echo "x=$X"'
add  33 'echo $((1 +)); echo after'
add  34 'echo ${undefinedvar?boom}; echo after'
add  35 'set -o nosuchoption; echo after'
add  36 'readonly rr=1; rr=2; echo after'
add  38 'readonly ii=1; for ii in a b; do :; done; echo after'
add  39 '. /no/such/file/xyz; echo after'
add  40 'eval "if"; echo after'
add  41 'unset 1badname; echo after'
add  44 'set -e; v=$(false; echo inner); echo "out v=$v rc=$?"'
add  45 'set -- a; shift 5; echo "rc=$?"'
add  48 'alias zz=yy; alias'
# #53 tests the POSIX *export display format*; grep the specific variable so
# the probe isn't polluted by bash-internal vars (BASH_EXECUTION_STRING) or
# any inherited env that incidentally matches.
add  53 'export EE=1; export | grep "^export EE="'
info 58 'kill -l' 'kill -l signal SET is OS-specific (Darwin lacks SIGSTKFLT/SIGPWR/realtime RTMIN..RTMAX); only the POSIX single-line format is asserted'
add  59 'kill -SIGTERM 2>&1 | head -1; echo "rc=$?"'
add  64 'ff() { :; }; gg=1; set | grep -E "^ff" | head -1; echo "---"'
add  65 'xx=hello; set | grep "^xx="'
add  68 'trap "echo x" INT; trap -p INT'
add  73 'readonly rv=1; unset -v rv; echo after'

# --- run bashy locally: capture stdout + success/fail (discard diagnostics) ---
declare -a BY_OUT BY_OK
for i in "${!NUMS[@]}"; do
  out=$("${CLEAN[@]}" "$BASHY" --posix -c "${SCRIPTS[$i]}" 2>/dev/null); rc=$?
  BY_OUT[$i]=$(printf '%s' "$out" | norm | tr '\n' '~')
  BY_OK[$i]=$([ "$rc" -eq 0 ] && echo ok || echo err)
done

# --- run bash 5.3 in one docker container, stdout + exit marker per probe ---
PROBES=$(for i in "${!NUMS[@]}"; do printf '%s\t%s\n' "$i" "${SCRIPTS[$i]}"; done)
RAW=$(printf '%s\n' "$PROBES" | docker run --rm -i -e HOME=/tmp bash:5.3 bash -c '
  tab=$(printf "\t")
  while IFS="$tab" read -r idx script; do
    echo "@@@P:$idx@@@"
    bash --posix -c "$script" 2>/dev/null
    echo "@@@X:$idx:$?@@@"
  done')
declare -a BH_OUT BH_OK
cur=""
while IFS= read -r line; do
  case "$line" in
    @@@P:*@@@)  cur=${line#@@@P:}; cur=${cur%@@@}; BH_OUT[$cur]="" ;;
    @@@X:*@@@)  m=${line#@@@X:}; m=${m%@@@}; ix=${m%%:*}; rc=${m#*:}
                BH_OK[$ix]=$([ "$rc" -eq 0 ] && echo ok || echo err); cur="" ;;
    *)          [ -n "$cur" ] && BH_OUT[$cur]+="$line"$'\n' ;;
  esac
done < <(printf '%s\n' "$RAW")
for i in "${!NUMS[@]}"; do
  # The inner $() trims trailing newlines, matching how bashy's stdout was
  # captured (command substitution strips them); then join with '~'.
  s=$(printf '%s' "${BH_OUT[$i]:-}" | norm)
  BH_OUT[$i]=$(printf '%s' "$s" | tr '\n' '~')
done

# --- compare on (stdout, success/fail) ---
match=0; diff=0; infon=0
for i in "${!NUMS[@]}"; do
  same=0
  if [ "${BY_OUT[$i]}" = "${BH_OUT[$i]:-}" ] && [ "${BY_OK[$i]}" = "${BH_OK[$i]:-}" ]; then
    same=1
  fi
  if [ -n "${NOTE[$i]}" ]; then
    if [ "$same" = 1 ]; then echo "INFO=  #${NUMS[$i]} — ${NOTE[$i]}"
    else echo "INFO~  #${NUMS[$i]} (expected host/OS difference) — ${NOTE[$i]}"; fi
    infon=$((infon+1)); continue
  fi
  if [ "$same" = 1 ]; then
    echo "MATCH  #${NUMS[$i]}"; match=$((match+1))
  else
    echo "DIFF   #${NUMS[$i]}"
    echo "   bashy: [${BY_OUT[$i]}] (${BY_OK[$i]})"
    echo "   bash:  [${BH_OUT[$i]:-}] (${BH_OK[$i]:-?})"
    diff=$((diff+1))
  fi
done
echo "=== $match match / $diff diff / $infon info / $((match+diff+infon)) probed ==="
[ "$diff" -eq 0 ]
