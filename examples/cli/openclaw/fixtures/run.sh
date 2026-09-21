#!/bin/sh
# Offline conformance fixtures for the OpenClaw bounded CLI profile.
#
# Candidate cases drive the real ycode binary against profile.yaml and hold
# it to the declared goldens and exit classes (usage=2, unsupported=4).
# Adapter cases drive main.bsh through the installed Bashy against a FAKE
# upstream executable: they are transport evidence only (argv projection,
# stream passthrough, exit-status propagation) and prove nothing about a
# real OpenClaw installation. No model calls are made.
set -eu

fixtures_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
profile="$fixtures_dir/../profile.yaml"
adapter="$fixtures_dir/../main.bsh"
golden="$fixtures_dir/golden"
bashy_bin=${BASHY_BIN:-$(command -v bashy)}
: "${YCODE_BIN:?set YCODE_BIN to the absolute candidate ycode executable}"
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

scratch=$(mktemp -d "${TMPDIR:-/tmp}/bashy-cli-openclaw.XXXXXX")
keep=1
trap 'if [ "$keep" -eq 0 ]; then rm -rf "$scratch"; else printf "fixture evidence: %s\n" "$scratch" >&2; fi' EXIT
trap 'exit 1' HUP INT TERM
export YCODE_CONFIG=
export OPENCLAW_CONFIG=
export XDG_CONFIG_HOME="$scratch/config"
export XDG_DATA_HOME="$scratch/data"
export BASHY_HOME="$scratch/bashy"
export BASHY_KB_DIR="$scratch/kb"
export BASHY_SKILLS_DIR="$scratch/skills"
export BASHY_HINTS=off
export LC_ALL=C
cd "$scratch"

passed=0
pass() {
    passed=$((passed + 1))
    printf 'PASS %s\n' "$1"
}

# Run the candidate against the profile; capture out/err/status.
status=0
run_ycode() {
    status=0
    "$YCODE_BIN" --file "$profile" "$@" >out 2>err </dev/null || status=$?
}

want_status() {
    label=$1
    want=$2
    if [ "$status" -ne "$want" ]; then
        printf '%s: status %s, want %s\n' "$label" "$status" "$want" >&2
        cat out err >&2
        exit 1
    fi
}

want_stderr_line() {
    label=$1
    line=$2
    printf '%s\n' "$line" >expected.err
    cmp -s expected.err err || {
        printf '%s: stderr differs from %s\n' "$label" "$line" >&2
        cat err >&2
        exit 1
    }
}

golden_case() {
    label=$1
    file=$2
    shift 2
    run_ycode "$@"
    want_status "$label" 0
    cmp -s "$golden/$file" out || {
        printf '%s: stdout differs from golden/%s\n' "$label" "$file" >&2
        diff "$golden/$file" out >&2 || true
        exit 1
    }
    pass "$label"
}

unsupported_case() {
    label=$1
    shift
    run_ycode "$@"
    want_status "$label" 4
    want_stderr_line "$label" 'error: command is unsupported by this CLI contract'
    pass "$label"
}

usage_case() {
    label=$1
    line=$2
    shift 2
    run_ycode "$@"
    want_status "$label" 2
    want_stderr_line "$label" "$line"
    pass "$label"
}

# --- candidate: strict validation ---------------------------------------
run_ycode validate
want_status validate 0
pass validate

# --- candidate: golden help, group default help, alias resolution -------
golden_case root-help root-help.txt --help
golden_case nested-help-model model-help.txt model --help
golden_case group-default-help model-help.txt model
golden_case help-command-nested resume-help.txt help resume
golden_case alias-tui start-help.txt tui --help
golden_case alias-terminal start-help.txt terminal --help
golden_case alias-chat start-help.txt chat --help
golden_case alias-models model-help.txt models --help
golden_case alias-onboard init-help.txt onboard --help

# --- candidate: supported dispatch --------------------------------------
golden_case model-list model-list.txt model list

run_ycode version
want_status version 0
[ -s out ] || { printf 'version: empty stdout\n' >&2; exit 1; }
pass version

run_ycode completion bash
want_status completion-bash 0
[ -s out ] || { printf 'completion-bash: empty stdout\n' >&2; exit 1; }
pass completion-bash

# --- candidate: declared unsupported behavior exits 4 --------------------
unsupported_case unsupported-root
unsupported_case unsupported-init init
unsupported_case unsupported-plan plan
unsupported_case unsupported-exit exit
unsupported_case unsupported-resume resume main
unsupported_case unsupported-model-status model status

# --- candidate: usage errors exit 2 --------------------------------------
usage_case usage-unknown-flag 'error: invalid command flags' --not-a-flag
usage_case usage-unknown-command 'error: openclaw expects 0 argument(s)' bogus
usage_case usage-too-many-args 'error: openclaw resume expects 0..1 argument(s)' resume a b
usage_case usage-completion-shell \
    'error: openclaw completion argument must be one of bash, zsh, fish, powershell' \
    completion elvish
usage_case usage-start-bad-flag 'error: invalid command flags' start --not-a-flag

# Flags the pinned OpenClaw source carries but the generic contract would
# silently ignore are NOT declared, so they are rejected as usage errors:
# the generic input route consumes only --session, and the generic inspect
# dispatch takes no flags.
usage_case usage-start-message 'error: invalid command flags' start --message hi
usage_case usage-start-local 'error: invalid command flags' start --local
usage_case usage-model-list-all 'error: invalid command flags' model list --all
usage_case usage-model-list-provider 'error: invalid command flags' model list --provider openai

# --- candidate: stdin/TTY routing ----------------------------------------
# start declares mode=auto stdin=false: interactive TTY selects the tui
# frontend; a non-TTY invocation is rejected as usage (input is required)
# and piped stdin is never consumed as a prompt.
usage_case stdin-routing-closed 'error: input is required' start
status=0
printf 'not-a-prompt\n' | "$YCODE_BIN" --file "$profile" start >out 2>err || status=$?
want_status stdin-routing-piped 2
want_stderr_line stdin-routing-piped 'error: input is required'
pass stdin-routing-piped
usage_case stdin-routing-flag-parsed 'error: input is required' start --session main

# --- adapter: fake-upstream transport evidence ---------------------------
# These cases substitute a fake OPENCLAW_BIN. They prove only that the
# Bash++ adapter projects argv, forwards streams and propagates exit
# status; they do not exercise a real OpenClaw.
fake="$scratch/fake-openclaw"
cat >"$fake" <<'EOF'
#!/bin/sh
printf 'argv:'
for a in "$@"; do printf ' %s' "$a"; done
printf '\n'
if [ ! -t 0 ]; then cat; fi
exit "${FAKE_STATUS:-0}"
EOF
chmod +x "$fake"

run_adapter() {
    status=0
    OPENCLAW_BIN="$fake" "$bashy_bin" --bashsharp "$adapter" "$@" >out 2>err </dev/null || status=$?
}

adapter_argv_case() {
    argv_label=$1
    argv_want=$2
    shift 2
    run_adapter "$@"
    want_status "$argv_label" 0
    printf '%s\n' "$argv_want" >expected.out
    cmp -s expected.out out || {
        printf '%s: argv projection differs\n' "$argv_label" >&2
        cat out err >&2
        exit 1
    }
    pass "$argv_label"
}

adapter_argv_case adapter-map-init 'argv: onboard --workspace w' init --workspace w
adapter_argv_case adapter-map-start 'argv: tui --session main' start --session main
adapter_argv_case adapter-map-model 'argv: models list' model list
adapter_argv_case adapter-passthrough 'argv: resume q' resume q

status=0
printf 'ping\n' | OPENCLAW_BIN="$fake" "$bashy_bin" --bashsharp "$adapter" resume >out 2>err || status=$?
want_status adapter-stdin-stream 0
printf 'argv: resume\nping\n' >expected.out
cmp -s expected.out out || { printf 'adapter-stdin-stream: stream transport differs\n' >&2; cat out err >&2; exit 1; }
pass adapter-stdin-stream

status=0
FAKE_STATUS=7 OPENCLAW_BIN="$fake" "$bashy_bin" --bashsharp "$adapter" version >out 2>err </dev/null || status=$?
want_status adapter-exit-status 7
pass adapter-exit-status

adapter_refuses() {
    label=$1
    verb=$2
    run_adapter "$verb"
    want_status "$label" 4
    grep -q "error: $verb is unsupported by the bounded OpenClaw profile" err || {
        printf '%s: missing unsupported message\n' "$label" >&2
        cat err >&2
        exit 1
    }
    [ -s out ] && { printf '%s: fake upstream was invoked\n' "$label" >&2; exit 1; }
    pass "$label"
}

adapter_refuses adapter-unsupported-plan plan
adapter_refuses adapter-unsupported-exit exit

printf 'PASS %s openclaw profile fixtures (adapter cases are fake-upstream transport evidence)\n' "$passed"
keep=0
