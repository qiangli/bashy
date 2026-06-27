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
# NB: deliberately NO `set -u`. The shell-under-test (bashy/sh) has a
# long-standing nounset bug — under `set -u`, assigning to a not-yet-set array
# element (`arr[$i]=…`) falsely errors "unbound variable", whereas real bash 5.3
# accepts it. This harness is interpreted by that shell, so `set -u` aborts it.
# Tracked as a separate sh conformance bug; does not affect the probes below.
BASHY=${BASHY:-./bin/bashy}

# Container runtime that provides the bash 5.3 oracle. Defaults to `docker`,
# but auto-falls back to `bashy podman` (the embedded rootless Podman on dev
# machines that have no Docker). Override with OCI="..." for anything else.
OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif command -v bashy  >/dev/null 2>&1; then OCI="bashy podman"
  else echo "error: no container runtime (need docker or bashy podman)" >&2; exit 2
  fi
fi

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

# --- batch 2 (2026-06-27): convert prose-asserted behaviors into live probes ---
# Each compares stdout + success/fail vs bash 5.3 --posix; a DIFF is a real
# finding to triage. These cover behaviors previously marked [x]-by-assertion
# in docs/posix-mode-behaviors.md but not exercised by a mechanical probe.
add   4 'alias do=BAD; for i in x y; do echo $i; done'
add   5 'alias g=echo; v=$(g hi); echo "$v"'
add   6 'time; echo "rc=$?"'
add  32 'f() { :; }; type f'
add  37 'readonly r=1; r=2 :; echo after'
add  42 'x=1 :; echo "x=[$x]"'
add  43 'command export ce=1; echo "ce=[$ce]"'
add  52 'shopt -s xpg_echo 2>/dev/null; echo "p\tq"'
add  66 '[ a \< b ]; echo "rc=$?"'
add  67 '[ -t ]; echo "rc=$?"'
add  69 'trap - 2; trap 2 EXIT 2>&1 | head -1; echo "rc=$?"'
# --- batch 3 (2026-06-27) ---
add   8 'set -- "x}y"; printf "%s\n" "${@}"'
add  13 'HOME=/H; v=~/x; echo "$v"'
add  60 'kill -0 99999999 2>/dev/null; echo "rc=$?"'
add  61 'printf "%.6f\n" 0.5'
add  62 'cd /; pwd'

# --- run bashy locally: capture stdout + success/fail (discard diagnostics) ---
declare -a BY_OUT BY_OK
for i in "${!NUMS[@]}"; do
  out=$("${CLEAN[@]}" "$BASHY" --posix -c "${SCRIPTS[$i]}" 2>/dev/null); rc=$?
  BY_OUT[$i]=$(printf '%s' "$out" | norm | tr '\n' '~')
  BY_OK[$i]=$([ "$rc" -eq 0 ] && echo ok || echo err)
done

# --- run bash 5.3 in one docker container, stdout + exit marker per probe ---
PROBES=$(for i in "${!NUMS[@]}"; do printf '%s\t%s\n' "$i" "${SCRIPTS[$i]}"; done)
RAW=$(printf '%s\n' "$PROBES" | $OCI run --rm -i -e HOME=/tmp bash:5.3 bash -c '
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
