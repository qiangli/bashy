#!/bin/sh
# Offline conformance fixture for the bounded Hermes Agent CLI example.
#
# Two independent checks, no model calls:
#   1. The declarative profile.yaml is strictly validated and its bounded CLI
#      surface is exercised through the FROZEN candidate (YCODE_BIN) against
#      committed goldens: root/nested/alias help, version, declared dispatch,
#      completion, and the usage(2) / unsupported(4) exit classes.
#   2. The thin Bash++ adapter (main.bsh) is executed through the installed
#      Bashy (BASHY_BIN). A FAKE upstream selected via HERMES_BIN proves ONLY
#      argv / stdin / stdout / stderr / exit-status transport and the bounded
#      `start`->`chat` alias rewrite. It is NOT evidence of real Hermes
#      behavior and makes no network or model calls.
#
# Usage:
#   env YCODE_BIN=/abs/ycode BASHY_BIN=/abs/bashy /bin/sh run.sh
set -eu

fixtures_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
profile="$fixtures_dir/../profile.yaml"
adapter="$fixtures_dir/../main.bsh"
goldens="$fixtures_dir/goldens"

: "${YCODE_BIN:?set YCODE_BIN to the absolute candidate ycode executable}"
bashy_bin=${BASHY_BIN:-$(command -v bashy)}
case "$YCODE_BIN" in
    /*) ;;
    *) printf '%s\n' 'YCODE_BIN must be an absolute path' >&2; exit 1 ;;
esac
case "$bashy_bin" in
    /*) ;;
    *) printf '%s\n' 'BASHY_BIN must be an absolute path' >&2; exit 1 ;;
esac
[ -x "$YCODE_BIN" ] || { printf 'not executable: %s\n' "$YCODE_BIN" >&2; exit 1; }
[ -x "$bashy_bin" ] || { printf 'not executable: %s\n' "$bashy_bin" >&2; exit 1; }

scratch=$(mktemp -d "${TMPDIR:-/tmp}/bashy-cli-hermes.XXXXXX")
keep=1
trap 'if [ "$keep" -eq 0 ]; then rm -rf "$scratch"; else printf "evidence: %s\n" "$scratch" >&2; fi' EXIT
trap 'exit 1' HUP INT TERM

# Deterministic, isolated runtime — goldens are generated under LC_ALL=C.
export LC_ALL=C
export HERMES_CONFIG=
export XDG_CONFIG_HOME="$scratch/config"
export XDG_DATA_HOME="$scratch/data"
export BASHY_HOME="$scratch/bashy"
export BASHY_HINTS=off
cd "$scratch"

passed=0

# --- 1a. strict validation of the declarative profile -----------------------
if ! "$YCODE_BIN" --file "$profile" validate >val.out 2>val.err; then
    printf 'FAIL validate: profile did not compile\n' >&2; cat val.err >&2; exit 1
fi
grep -q '^valid: hermes' val.out || { printf 'FAIL validate: unexpected output\n' >&2; cat val.out >&2; exit 1; }
passed=$((passed + 1)); printf 'PASS validate\n'

# --- 1b. candidate goldens (stdout, stderr and exit class) ------------------
golden() {
    name=$1; want_exit=$2; shift 2
    status=0
    "$YCODE_BIN" --file "$profile" "$@" >cand.out 2>cand.err || status=$?
    if [ "$status" -ne "$want_exit" ]; then
        printf 'FAIL golden %s: exit=%s want=%s\n' "$name" "$status" "$want_exit" >&2
        cat cand.err >&2; exit 1
    fi
    cmp -s "$goldens/$name.out" cand.out || {
        printf 'FAIL golden %s: stdout differs\n' "$name" >&2
        diff "$goldens/$name.out" cand.out 2>&1 | head -40 >&2; exit 1; }
    cmp -s "$goldens/$name.err" cand.err || {
        printf 'FAIL golden %s: stderr differs\n' "$name" >&2
        diff "$goldens/$name.err" cand.err 2>&1 | head -40 >&2; exit 1; }
    passed=$((passed + 1)); printf 'PASS golden %s (exit %s)\n' "$name" "$want_exit"
}

golden help            0 --help
golden help_chat       0 chat --help
golden help_alias      0 start --help
golden help_completion 0 completion --help
golden version         0 version
golden unsupported_chat   4 chat
golden unsupported_model  4 model
golden unsupported_prompt 4 hello there
golden usage_bad_shell 2 completion telnet
golden usage_bad_flag  2 chat --nope
golden completion_bash 0 completion bash

# --- 2. adapter transport through installed Bashy + FAKE upstream -----------
# The fake upstream records its argv and stdin and echoes deterministic
# markers; it proves transport only.
fake="$scratch/fake-hermes"
argv_file="$scratch/argv.txt"
stdin_file="$scratch/stdin.txt"
cat >"$fake" <<EOF
#!/bin/sh
: >"$argv_file"
for a in "\$@"; do printf '%s\n' "\$a" >>"$argv_file"; done
cat >"$stdin_file"
printf 'OUT:%s\n' "\${1:-}"
printf 'ERR:%s\n' "\${1:-}" >&2
exit "\${FAKE_EXIT:-0}"
EOF
chmod +x "$fake"
export HERMES_BIN="$fake"

# argv passthrough (verbatim forwarding)
printf '' | "$bashy_bin" --bashsharp "$adapter" chat -q hi >/dev/null 2>&1 || true
printf 'chat\n-q\nhi\n' >want_argv
cmp -s want_argv "$argv_file" || { printf 'FAIL transport argv passthrough\n' >&2; diff want_argv "$argv_file" >&2; exit 1; }
passed=$((passed + 1)); printf 'PASS transport argv passthrough (fake upstream)\n'

# bounded alias rewrite start -> chat
printf '' | "$bashy_bin" --bashsharp "$adapter" start -q hi >/dev/null 2>&1 || true
printf 'chat\n-q\nhi\n' >want_alias
cmp -s want_alias "$argv_file" || { printf 'FAIL transport alias start->chat\n' >&2; diff want_alias "$argv_file" >&2; exit 1; }
passed=$((passed + 1)); printf 'PASS transport alias start->chat (fake upstream)\n'

# stdin passthrough
printf 'PING' | "$bashy_bin" --bashsharp "$adapter" chat >/dev/null 2>&1 || true
printf 'PING' >want_stdin
cmp -s want_stdin "$stdin_file" || { printf 'FAIL transport stdin passthrough\n' >&2; exit 1; }
passed=$((passed + 1)); printf 'PASS transport stdin passthrough (fake upstream)\n'

# stdout / stderr passthrough
printf '' | "$bashy_bin" --bashsharp "$adapter" chat >o.out 2>o.err || true
grep -q '^OUT:chat$' o.out || { printf 'FAIL transport stdout passthrough\n' >&2; cat o.out >&2; exit 1; }
grep -q '^ERR:chat$' o.err || { printf 'FAIL transport stderr passthrough\n' >&2; cat o.err >&2; exit 1; }
passed=$((passed + 1)); printf 'PASS transport stdout/stderr passthrough (fake upstream)\n'

# exit-status passthrough
status=0
FAKE_EXIT=7 "$bashy_bin" --bashsharp "$adapter" chat </dev/null >/dev/null 2>&1 || status=$?
[ "$status" -eq 7 ] || { printf 'FAIL transport status passthrough: got %s want 7\n' "$status" >&2; exit 1; }
passed=$((passed + 1)); printf 'PASS transport status passthrough (fake upstream)\n'

printf 'PASS %s hermes-agent fixture checks\n' "$passed"
keep=0
