#!/bin/sh
# Offline, deterministic fixtures for the bounded mini-swe-agent example.
#
# Three independent surfaces, no network and no model call anywhere:
#
#   1. CLI CONTRACT (ycode)   — the frozen ycode binary strictly validates
#      profile.yaml and renders/dispatches the declared CLI: golden help, the
#      supported ops (version/completion), the declared-unsupported ops (exit 4),
#      and usage errors (exit 2). ycode's frozen dispatch cannot launch a mini
#      loop, so start/run themselves exit unsupported (4) here — that is the
#      documented example-local limit, not parity.
#
#   2. ADAPTER TRANSPORT (bashy) — main.bpp is driven against a FAKE upstream
#      `mini`. These cases are transport evidence ONLY (argv projection, stream
#      passthrough, exit-status propagation, unsupported-verb refusal); they
#      prove nothing about a real mini-swe-agent installation.
#
#   3. LIFECYCLE HARNESS (python) — the self-contained, stdlib-only bounded
#      harness replays each scenario end-to-end (real agent loop + real local
#      shell actions + scripted replay model) and its NORMALIZED result envelope
#      is held to the committed golden. This is the actual lifecycle/exit-outcome
#      parity: submitted, cost/step limits, and repeated format errors, WITH the
#      executed actions and the successful submission. Replay-only, never live.
#
# Requirements: an absolute YCODE_BIN (frozen ycode), an absolute BASHY_BIN (or
# `bashy` on PATH), and python3.
set -eu

fixtures_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
example_dir=$(CDPATH= cd -- "$fixtures_dir/.." && pwd)
profile="$example_dir/profile.yaml"
adapter="$example_dir/main.bpp"
harness_dir="$example_dir/harness"
cli_golden="$fixtures_dir/cli"
scenarios="$fixtures_dir/scenarios"
result_golden="$fixtures_dir/goldens"

bashy_bin=${BASHY_BIN:-$(command -v bashy || true)}
python_bin=${PYTHON_BIN:-$(command -v python3 || true)}
: "${YCODE_BIN:?set YCODE_BIN to the absolute frozen ycode executable}"
case "$YCODE_BIN" in
    /*) ;;
    *) printf '%s\n' 'YCODE_BIN must be an absolute path' >&2; exit 1 ;;
esac
case "$bashy_bin" in
    /*) ;;
    *) printf '%s\n' 'BASHY_BIN must be an absolute path (or bashy on PATH)' >&2; exit 1 ;;
esac
[ -x "$YCODE_BIN" ] || { printf 'not executable: %s\n' "$YCODE_BIN" >&2; exit 1; }
[ -x "$bashy_bin" ] || { printf 'not executable: %s\n' "$bashy_bin" >&2; exit 1; }
[ -n "$python_bin" ] || { printf '%s\n' 'python3 not found (set PYTHON_BIN)' >&2; exit 1; }

scratch=$(mktemp -d "${TMPDIR:-/tmp}/bashy-cli-mini.XXXXXX")
keep=1
trap 'if [ "$keep" -eq 0 ]; then rm -rf "$scratch"; else printf "fixture evidence: %s\n" "$scratch" >&2; fi' EXIT
trap 'exit 1' HUP INT TERM
export XDG_CONFIG_HOME="$scratch/config"
export XDG_DATA_HOME="$scratch/data"
export BASHY_HOME="$scratch/bashy"
export BASHY_HINTS=off
export LC_ALL=C
cd "$scratch"

passed=0
pass() { passed=$((passed + 1)); printf 'PASS %s\n' "$1"; }

status=0
run_ycode() { status=0; "$YCODE_BIN" --file "$profile" "$@" >out 2>err </dev/null || status=$?; }

want_status() {
    if [ "$status" -ne "$2" ]; then
        printf '%s: status %s, want %s\n' "$1" "$status" "$2" >&2; cat out err >&2; exit 1
    fi
}
want_stderr_line() {
    printf '%s\n' "$2" >expected.err
    cmp -s expected.err err || { printf '%s: stderr differs from %s\n' "$1" "$2" >&2; cat err >&2; exit 1; }
}
golden_case() {
    label=$1; file=$2; shift 2
    run_ycode "$@"; want_status "$label" 0
    cmp -s "$cli_golden/$file" out || {
        printf '%s: stdout differs from cli/%s\n' "$label" "$file" >&2
        diff "$cli_golden/$file" out >&2 || true; exit 1
    }
    pass "$label"
}
unsupported_case() {
    label=$1; shift
    run_ycode "$@"; want_status "$label" 4
    want_stderr_line "$label" 'error: command is unsupported by this CLI contract'
    pass "$label"
}
usage_case() {
    label=$1; line=$2; shift 2
    run_ycode "$@"; want_status "$label" 2
    want_stderr_line "$label" "$line"
    pass "$label"
}

# --- 1. CLI CONTRACT (ycode) --------------------------------------------
run_ycode validate; want_status validate 0
printf '%s' "$(cat out)" | grep -q '^valid: mini' || { printf 'validate: unexpected output\n' >&2; cat out >&2; exit 1; }
pass validate

golden_case root-help root-help.txt --help
golden_case start-help start-help.txt help start

run_ycode version; want_status version 0
[ -s out ] || { printf 'version: empty stdout\n' >&2; exit 1; }
pass version

run_ycode completion bash; want_status completion-bash 0
[ -s out ] || { printf 'completion-bash: empty stdout\n' >&2; exit 1; }
pass completion-bash

# ycode's frozen dispatch cannot launch a mini loop; the lifecycle verbs and the
# not-in-source verbs are all declared unsupported and exit 4.
unsupported_case unsupported-start start
unsupported_case unsupported-run run
unsupported_case unsupported-bare-task -t hello
unsupported_case unsupported-init init
unsupported_case unsupported-model model
unsupported_case unsupported-plan plan
unsupported_case unsupported-resume resume
unsupported_case unsupported-exit exit

usage_case usage-unknown-flag 'error: invalid command flags' --not-a-flag
usage_case usage-unknown-command 'error: mini expects 0 argument(s)' bogus
usage_case usage-resume-args 'error: mini resume expects 0 argument(s)' resume extra
usage_case usage-completion-shell \
    'error: mini completion argument must be one of bash, zsh, fish, powershell' \
    completion elvish

# --- 2. ADAPTER TRANSPORT (bashy, fake upstream) ------------------------
fake="$scratch/fake-mini"
cat >"$fake" <<'EOF'
#!/bin/sh
printf 'argv:'
for a in "$@"; do printf ' %s' "$a"; done
printf '\n'
if [ ! -t 0 ]; then cat; fi
exit "${FAKE_STATUS:-0}"
EOF
chmod +x "$fake"

run_adapter() { status=0; MINI_BIN="$fake" "$bashy_bin" --bashpp "$adapter" "$@" >out 2>err </dev/null || status=$?; }
adapter_argv_case() {
    label=$1; want=$2; shift 2
    run_adapter "$@"; want_status "$label" 0
    printf '%s\n' "$want" >expected.out
    cmp -s expected.out out || { printf '%s: argv projection differs\n' "$label" >&2; cat out err >&2; exit 1; }
    pass "$label"
}

adapter_argv_case adapter-start 'argv: -t fix' start -t fix
adapter_argv_case adapter-run 'argv: -m gpt' run -m gpt
adapter_argv_case adapter-passthrough 'argv: -t hello' -t hello

status=0
printf 'ping\n' | MINI_BIN="$fake" "$bashy_bin" --bashpp "$adapter" run >out 2>err || status=$?
want_status adapter-stdin-stream 0
printf 'argv:\nping\n' >expected.out
cmp -s expected.out out || { printf 'adapter-stdin-stream: stream transport differs\n' >&2; cat out err >&2; exit 1; }
pass adapter-stdin-stream

status=0
FAKE_STATUS=7 MINI_BIN="$fake" "$bashy_bin" --bashpp "$adapter" --version >out 2>err </dev/null || status=$?
want_status adapter-exit-status 7
pass adapter-exit-status

adapter_refuses() {
    label=$1; verb=$2
    run_adapter "$verb"; want_status "$label" 4
    grep -q "error: $verb is unsupported by the bounded mini-swe-agent profile" err || {
        printf '%s: missing unsupported message\n' "$label" >&2; cat err >&2; exit 1
    }
    [ -s out ] && { printf '%s: fake upstream was invoked\n' "$label" >&2; exit 1; }
    pass "$label"
}
adapter_refuses adapter-unsupported-plan plan
adapter_refuses adapter-unsupported-exit exit

# --- 3. LIFECYCLE HARNESS (python replay) -------------------------------
harness_case() {
    name=$1
    status=0
    ( cd "$harness_dir" && "$python_bin" -m minisweagent_bounded.runner "$scenarios/$name.json" ) >out 2>err || status=$?
    want_status "harness-$name" 0
    cmp -s "$result_golden/$name.json" out || {
        printf 'harness-%s: normalized envelope differs from goldens/%s.json\n' "$name" "$name" >&2
        diff "$result_golden/$name.json" out >&2 || true; exit 1
    }
    pass "harness-$name"
}
harness_case submit-success
harness_case cost-limit
harness_case step-limit
harness_case repeated-format-error
harness_case format-error-recovers

# The bundled python unit tests are the harness's own regression gate.
status=0
( cd "$harness_dir" && "$python_bin" -m unittest discover -s tests -q ) >out 2>err || status=$?
want_status harness-unittest 0
pass harness-unittest

printf 'PASS %s mini-swe-agent fixtures (adapter cases are fake-upstream transport evidence; harness is replay-only)\n' "$passed"
keep=0
