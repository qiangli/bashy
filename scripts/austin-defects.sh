#!/usr/bin/env bash
# austin-defects.sh — clean-room differential probe for classic POSIX shell
# corner cases clarified by Austin Group defect/interpretation reports.
#
# The Austin Group (the joint POSIX/Open Group working group that maintains
# IEEE Std 1003.1 / the Single UNIX Specification) publishes defect reports and
# interpretations that pin down the *intended* behavior of XCU §2 (Shell
# Command Language) on points where the base text was ambiguous: field
# splitting around IFS white/non-white delimiters, the `:` vs no-`:` parameter
# expansion forms, quote-removal ordering, arithmetic sign/short-circuit rules,
# `case` pattern semantics, tilde-in-assignment, `read` backslash handling,
# special- vs regular-builtin assignment persistence, and so on.
#
# This harness asserts that `bashy --posix` agrees with GNU bash 5.3 `--posix`
# (the reference oracle, run via a `bash:5.3` container since macOS ships bash
# 3.2) on those points. The probes below are authored CLEAN-ROOM from POSIX
# XCU knowledge — no text is copied from any licensed/GPL conformance suite.
# Each probe encodes the behavior an Austin Group interpretation settled on,
# expressed as a tiny `-c` script whose *observable output* discriminates the
# correct answer.
#
# Like scripts/posix-parity.sh, conformance here is SEMANTIC, not byte-exact
# mimicry. For each probe we compare:
#   (a) stdout (the observable program output), normalized; and
#   (b) whether the shell SUCCEEDED or FAILED (exit 0 vs non-zero).
# We do NOT compare diagnostic wording or the specific exit-CODE value — POSIX
# mandates neither. The shell's own stderr is discarded; a probe that needs to
# observe an error folds it into stdout itself.
#
# Probes whose result is inherently host/OS-specific are marked INFO and
# excluded from the pass/fail count. Every probe here is pure shell semantics
# (no filesystem mutation, no signals, no OS-specific data), so there are
# currently none — the hook is kept for parity with the template.
#
# Usage: scripts/austin-defects.sh [--candidate-only | --oracle-bash /path/to/bash-5.3]
# A local upstream GNU Bash 5.3 oracle avoids a container on native hosts.
# Candidate-only mode records probe execution without scoring conformance.
# Exit: 0 iff every non-INFO probe matches  (0-gate suite — plugs into
#       scripts/posix-certdryrun.sh, which expects this contract).
# NB: deliberately NO `set -u` — same long-standing sh nounset/array bug noted
# in posix-parity.sh; this harness is interpreted by that shell.
BASHY=${BASHY:-./bin/bashy}
ORACLE_BASH=
CANDIDATE_ONLY=0
case "${1:-}" in
  '') ;;
  --candidate-only) CANDIDATE_ONLY=1 ;;
  --oracle-bash)
    [ "$#" -eq 2 ] && [ -x "$2" ] || {
      echo "austin-defects: --oracle-bash needs an executable path" >&2; exit 2;
    }
    ORACLE_BASH=$2 ;;
  *) echo "usage: $0 [--candidate-only | --oracle-bash /path/to/bash-5.3]" >&2; exit 2 ;;
esac
if [ "$CANDIDATE_ONLY" = 1 ] || [ -n "$ORACLE_BASH" ]; then
  "$BASHY" check --prepare "$0" || {
    echo "austin-defects: preload failed; no probes started" >&2; exit 2
  }
  "$BASHY" -c 'for tool in env sed tr grep head awk; do
    command -v "$tool" >/dev/null || { printf "missing harness utility: %s\n" "$tool" >&2; exit 1; }
  done' || {
    echo "austin-defects: Bashy userland preload incomplete; no probes started" >&2; exit 2
  }
  echo "austin-defects: preload complete: harness utilities resolved in Bashy"
fi
if [ -n "$ORACLE_BASH" ]; then
  version=$("$ORACLE_BASH" --version | head -1)
  case "$version" in
    'GNU bash, version 5.3.'*'-release '*) ;;
    *) echo "austin-defects: local oracle must be upstream GNU Bash 5.3: $version" >&2; exit 2 ;;
  esac
  if [ "$ORACLE_BASH" -ef "$BASHY" ]; then
    echo "austin-defects: oracle and candidate are the same executable" >&2; exit 2
  fi
  echo "austin-defects: local oracle: $version ($ORACLE_BASH)"
fi

# Container runtime that provides the bash 5.3 oracle. Defaults to `docker`,
# auto-falls back to `bashy podman` (embedded rootless Podman on dev machines
# without Docker). Override with OCI="..." for anything else.
OCI=${OCI:-}
if [ -z "$OCI" ] && [ -z "$ORACLE_BASH" ] && [ "$CANDIDATE_ONLY" = 0 ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif [ -n "${BASHY:-}" ]; then OCI="$BASHY podman"
  elif command -v bashy  >/dev/null 2>&1; then OCI="bashy podman"
  else echo "error: no container runtime (need docker or bashy podman)" >&2; exit 2
  fi
fi

# Run bashy in a clean, minimal environment so host variables don't leak into
# its expansions while the container bash sees a pristine env.
CLEAN=(env -i HOME=/tmp PATH=/usr/bin:/bin)

# Normalize the program-name prefix and line numbers so only semantic
# differences in stdout remain.
norm() { sed -E 's#[^ ]*(bashy|bash):#SH:#g; s/line [0-9]+/line N/g'; }

# add ID 'script'         — a strict conformance probe (counted)
# info ID 'script' 'note' — host/OS-specific; reported but not counted
declare -a NUMS SCRIPTS NOTE
add()  { NUMS+=("$1"); SCRIPTS+=("$2"); NOTE+=(""); }
info() { NUMS+=("$1"); SCRIPTS+=("$2"); NOTE+=("$3"); }

# --- field splitting / IFS (XCU §2.6.5; the IFS white/non-white-delimiter rule
#     is one of the most-interpreted corners of the spec) ---
# Empty IFS disables field splitting entirely: the expansion is one field.
add fs-empty    'IFS=; var="a b c"; set -- $var; echo "n=$#"'
# A non-white IFS delimiter is significant on its own: adjacent delimiters
# yield an empty field between them.
add fs-adjacent 'IFS=:; var="a::b"; set -- $var; echo "n=$#"'
# Trailing IFS *white space* does NOT create a trailing empty field.
add fs-trailws  'var="a b  "; set -- $var; echo "n=$#"'
# Trailing IFS *non-white* delimiter: Austin Group settled that a single
# trailing non-white delimiter does not create a trailing empty field.
add fs-trailnw  'IFS=:; var="a:b:"; set -- $var; echo "n=$#"'
# "$*" joins fields with the first character of IFS.
add fs-star     'IFS=-; set -- a b c; echo "[$*]"'
# "$@" always joins with a space regardless of IFS.
add fs-at       'IFS=-; set -- a b c; echo "[$@]"'
# Splitting applies to the result of expansion, not to quoted text.
add fs-quoted   'IFS=:; v="a:b"; echo $v; echo "$v"'

# --- parameter expansion (XCU §2.6.2): the ':' vs no-':' distinction and
#     longest/shortest pattern removal are perennial interpretation topics ---
# ${x:-w} treats null OR unset as "use w"; ${x-w} only treats unset that way.
add pe-colonset 'x=; echo "[${x:-D}][${x-D}]"'
add pe-unsetdf  'unset x; echo "[${x:-D}][${x-D}]"'
# ${x:=w} assigns w to x as a side effect (and only for unset/null).
add pe-assign   'unset x; : "${x:=val}"; echo "[$x]"'
# ${x:+w} substitutes w only when x is set and non-null.
add pe-altset   'x=s; echo "[${x:+A}]"; unset x; echo "[${x:+A}]"'
# ${#x} is the length in characters of the value of x.
add pe-length   'x=hello; echo "${#x}"'
# %/%% remove shortest/longest matching suffix; #/## shortest/longest prefix.
add pe-suffix   'x=aXbXc; echo "[${x%X*}][${x%%X*}]"'
add pe-prefix   'x=aXbXc; echo "[${x#*X}][${x##*X}]"'
# Nested expansion in the word part is evaluated normally.
add pe-nested   'unset x z; echo "[${x:-${z:-bar}}]"'

# --- set -u / unset reference (XCU §2.5.3, §special-parameters) ---
# Referencing an unset variable under `set -u` is an error; a non-interactive
# shell exits, so the following command does not run.
add u-var       'unset x; set -u; echo "${x}"; echo after'
# Special parameters $#, $? are always set: `set -u` does not error on them.
add u-special   'set -u; set --; echo "c=$# r=$?"'
# An unset positional under `set -u` is an error (after the set one prints).
add u-pos       'set -u; set -- a; echo "$1"; echo "${2}"; echo after'

# --- arithmetic ($(( )), XCU §2.6.4 / §1.1.2 integer semantics) ---
# Base prefixes: 0x hex, leading-0 octal, base#digits.
add ar-base     'echo "$((0x10)) $((010)) $((2#101))"'
# C operator precedence; parentheses override.
add ar-prec     'echo "$((2+3*4)) $(((2+3)*4))"'
# Division truncates toward zero; % sign follows the dividend.
add ar-divmod   'echo "$((-7/2)) $((7%-2)) $((-7%2))"'
# && short-circuits: the right operand (a division by zero) is not evaluated.
add ar-short    'echo "$((0 && (1/0)))"'
# Assignment inside $(( )) is an operator with a side effect.
add ar-comma    'echo "$((y=5, y+1))"; echo "[$y]"'
# Ternary conditional.
add ar-ternary  'echo "$((1?2:3)) $((0?2:3))"'

# --- case pattern matching (XCU §2.9.4.3) ---
# Bracket expression in a case pattern.
add case-brkt   'case abc in [ab]*) echo br;; *) echo no;; esac'
# A quoted pattern is a literal: "*" matches only a literal asterisk.
add case-quote  'case x in "*") echo lit;; *) echo glob;; esac; s="*"; case "$s" in "*") echo lit;; *) echo glob;; esac'
# First matching clause wins; later duplicate patterns are not reached.
add case-first  'case b in b) echo one;; b) echo two;; esac'

# --- tilde expansion in assignments (XCU §2.6.1): a tilde-prefix is expanded
#     at the start of an assignment word and after each unquoted ':'.
add tilde-asn   'HOME=/h; x=a:~/d; echo "$x"'

# --- read builtin field/backslash handling (XCU read, §2.6.5 splitting) ---
# Excess fields go to the last variable, with leading/trailing IFS trimmed.
add read-rest   'printf "%s\n" "a b c d" | { read x y; echo "[$x][$y]"; }'
# Without -r, backslash is an escape: `a\ b` reads as one field `a b`.
add read-bsl    'printf "%s\n" "a\\ b" | { read x y; echo "[$x][$y]"; }'
# With -r, backslash is literal.
add read-raw    'printf "%s\n" "a\\ b" | { read -r x y; echo "[$x][$y]"; }'

# --- pipeline / ! / command-substitution exit status (XCU §2.9.2, §2.12) ---
# A pipeline's status is that of its last command.
add pipe-last   'false | true; echo "$?"'
# `! pipeline` logically negates the exit status.
add bang-neg    '! false; echo "$?"; ! true; echo "$?"'
# Command substitution's status is the status of the substituted command.
add cs-status   'x=$(exit 3); echo "$?"'
# Command substitution strips trailing newlines from the captured output.
add cs-trim     'x=$(printf "a\n\n\n"); echo "[$x]"'

# --- special vs regular builtins (XCU §2.14): a variable assignment preceding
#     a SPECIAL builtin persists in the current environment; preceding a
#     REGULAR builtin (or external) it is transient. ---
add sb-persist  'x=1; x=2 :; echo "[$x]"'
add rb-trans    'x=1; x=2 true; echo "[$x]"'

# --- run bashy locally: capture stdout + success/fail (discard diagnostics) ---
declare -a BY_OUT BY_OK BY_RC
for i in "${!NUMS[@]}"; do
  if [ "$CANDIDATE_ONLY" = 1 ]; then
    out=$("${CLEAN[@]}" "$BASHY" --posix -c "${SCRIPTS[$i]}" 2>&1); rc=$?
  else
    out=$("${CLEAN[@]}" "$BASHY" --posix -c "${SCRIPTS[$i]}" 2>/dev/null); rc=$?
  fi
  BY_OUT[$i]=$(printf '%s' "$out" | norm | tr '\n' '~')
  BY_OK[$i]=$([ "$rc" -eq 0 ] && echo ok || echo err)
  BY_RC[$i]=$rc
done

if [ "$CANDIDATE_ONLY" = 1 ]; then
  for i in "${!NUMS[@]}"; do
    printf 'EXECUTED #%s rc=%s stdout+stderr=[%s]\n' "${NUMS[$i]}" "${BY_RC[$i]}" "${BY_OUT[$i]}"
  done
  echo "=== ${#NUMS[@]} candidate probes executed / no oracle comparison ==="
  exit 0
fi

# --- run the independent bash 5.3 reference ---
declare -a BH_OUT_N BH_OK_N
if [ -n "$ORACLE_BASH" ]; then
  for i in "${!NUMS[@]}"; do
    out=$("${CLEAN[@]}" "$ORACLE_BASH" --posix -c "${SCRIPTS[$i]}" 2>/dev/null); rc=$?
    BH_OUT_N[$i]=$(printf '%s' "$out" | norm | tr '\n' '~')
    BH_OK_N[$i]=$([ "$rc" -eq 0 ] && echo ok || echo err)
  done
else
PROBES=$(for i in "${!NUMS[@]}"; do printf '%s\t%s\n' "$i" "${SCRIPTS[$i]}"; done)
RAW=$(printf '%s\n' "$PROBES" | $OCI run --rm -i -e HOME=/tmp bash:5.3 bash -c '
  tab=$(printf "\t")
  while IFS="$tab" read -r idx script; do
    echo "@@@P:$idx@@@"
    bash --posix -c "$script" 2>/dev/null
    echo "@@@X:$idx:$?@@@"
  done')
OCI_RC=$?
if [ "$OCI_RC" -ne 0 ]; then
  echo "austin-defects: POSIX oracle invocation failed (exit $OCI_RC); comparison is blocked" >&2
  exit 2
fi
declare -a BH_OUT BH_OK
cur=""
while IFS= read -r line; do
  case "$line" in
    @@@P:*@@@)  cur=${line#@@@P:}; cur=${cur%@@@}; BH_OUT[$cur]="" ;;
    @@@X:*@@@)  m=${line#@@@X:}; m=${m%@@@}; ix=${m%%:*}; rc=${m##*:}
                BH_OK[$ix]=$([ "$rc" -eq 0 ] && echo ok || echo err); cur="" ;;
    *)          [ -n "$cur" ] && BH_OUT[$cur]+="$line"$'\n' ;;
  esac
done < <(printf '%s\n' "$RAW")
# Map container marker index (numeric position) back to probe ID for compare.
# We keyed BH_OUT/BH_OK by the same loop index $i used to print PROBES, so a
# parallel index walk lines them up regardless of the (string) probe IDs.
n=0
for i in "${!NUMS[@]}"; do
  s=$(printf '%s' "${BH_OUT[$n]:-}" | norm)
  BH_OUT_N[$i]=$(printf '%s' "$s" | tr '\n' '~')
  BH_OK_N[$i]=${BH_OK[$n]:-?}
  n=$((n+1))
done
fi

# --- compare on (stdout, success/fail) ---
match=0; diff=0; infon=0
for i in "${!NUMS[@]}"; do
  same=0
  if [ "${BY_OUT[$i]}" = "${BH_OUT_N[$i]:-}" ] && [ "${BY_OK[$i]}" = "${BH_OK_N[$i]:-}" ]; then
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
    echo "   bash:  [${BH_OUT_N[$i]:-}] (${BH_OK_N[$i]:-?})"
    diff=$((diff+1))
  fi
done
echo "=== $match match / $diff diff / $infon info / $((match+diff+infon)) probed ==="
[ "$diff" -eq 0 ]
