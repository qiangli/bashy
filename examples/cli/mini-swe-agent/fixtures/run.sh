#!/bin/sh
# Shared, deterministic, offline fixture suite for the bounded mini-swe-agent
# example. No paid/network model call anywhere (the live case uses a loopback
# fake provider). Six surfaces:
#
#   1. CLI CONTRACT (ycode) — the frozen ycode compiler strictly validates and
#      renders profile.yaml; the run itself returns the exit-4 bridge signal and
#      usage errors return 2.
#   2. ENTRYPOINT PARITY — the shell entrypoint (main.bpp) and the YAML bridge
#      (yaml-run.sh) run the SAME local loop and produce byte-identical
#      normalized envelopes, equal to the committed goldens, across every
#      scenario (real actions + successful submission + limit/format outcomes).
#   3. INTERACTION + INTERRUPTION — real PTY input (confirm/human) and a real
#      SIGINT drive the ACTUAL cli.py; the trajectory and exit code are checked
#      and the child process group is reaped (no orphan).
#   4. LIVE TRANSPORT — the LiveModel's real urllib transport against a loopback
#      OpenAI-compatible provider.
#   5. FAIL-CLOSED + VALIDATION — non-interactive confirm/human and missing task
#      fail closed (never approved by piped newlines); bad budgets/classes/config
#      fail explicitly.
#   6. UNIT TESTS — the harness's own python regression suite.
#
# Requires an absolute YCODE_BIN, a BASHY_BIN (or bashy on PATH), and python3.
set -eu

fixtures_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
example_dir=$(CDPATH= cd -- "$fixtures_dir/.." && pwd)
profile="$example_dir/profile.yaml"
adapter="$example_dir/main.bpp"
yaml_entry="$example_dir/yaml-run.sh"
harness_dir="$example_dir/harness"
cli_golden="$fixtures_dir/cli"
scenarios="$fixtures_dir/scenarios"
result_golden="$fixtures_dir/goldens"

bashy_bin=${BASHY_BIN:-$(command -v bashy || true)}
python_bin=${PYTHON_BIN:-$(command -v python3 || true)}
: "${YCODE_BIN:?set YCODE_BIN to the absolute frozen ycode executable}"
case "$YCODE_BIN" in /*) ;; *) echo 'YCODE_BIN must be absolute' >&2; exit 1 ;; esac
case "$bashy_bin" in /*) ;; *) echo 'BASHY_BIN must be absolute (or bashy on PATH)' >&2; exit 1 ;; esac
[ -x "$YCODE_BIN" ] || { echo "not executable: $YCODE_BIN" >&2; exit 1; }
[ -x "$bashy_bin" ] || { echo "not executable: $bashy_bin" >&2; exit 1; }
[ -n "$python_bin" ] || { echo 'python3 not found (set PYTHON_BIN)' >&2; exit 1; }

export BASHY_BIN="$bashy_bin"
export YCODE_BIN
export PYTHON_BIN="$python_bin"
export BASHY_HINTS=off
export LC_ALL=C

scratch=$(mktemp -d "${TMPDIR:-/tmp}/bashy-cli-mini.XXXXXX")
keep=1
trap 'if [ "$keep" -eq 0 ]; then rm -rf "$scratch"; else printf "fixture evidence: %s\n" "$scratch" >&2; fi' EXIT
trap 'exit 1' HUP INT TERM
export MSWEA_WORKDIR="$scratch"

passed=0
pass() { passed=$((passed + 1)); printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2" >&2; exit 1; }

status=0
run_ycode() { status=0; "$YCODE_BIN" --file "$profile" "$@" >"$scratch/out" 2>"$scratch/err" </dev/null || status=$?; }
want_status() { [ "$status" -eq "$2" ] || { cat "$scratch/out" "$scratch/err" >&2; fail "$1" "status $status want $2"; }; }

# --- 1. CLI CONTRACT (ycode) --------------------------------------------
run_ycode validate; want_status validate 0
grep -q '^valid: mini' "$scratch/out" || fail validate "unexpected output"
pass validate

run_ycode --help; want_status root-help 0
cmp -s "$cli_golden/root-help.txt" "$scratch/out" || { diff "$cli_golden/root-help.txt" "$scratch/out" >&2 || true; fail root-help "golden mismatch"; }
pass root-help

run_ycode version; want_status version 0; [ -s "$scratch/out" ] || fail version empty; pass version
run_ycode completion bash; want_status completion 0; [ -s "$scratch/out" ] || fail completion empty; pass completion

run_ycode -t "do it"; want_status run-bridge-exit 4; pass run-bridge-exit4        # the bridge signal
run_ycode --not-a-flag; want_status usage-bad-flag 2; pass usage-bad-flag
run_ycode completion elvish; want_status usage-bad-shell 2; pass usage-bad-shell

# --- 2. ENTRYPOINT PARITY (shell == yaml == golden) ---------------------
parity_case() {
    name=$1
    shell_work=$(mktemp -d "$scratch/${name}-shell.XXXXXX")
    yaml_work=$(mktemp -d "$scratch/${name}-yaml.XXXXXX")
    # The envelope is emitted on stdout even when the terminal outcome is a
    # non-submit (exit 1); `|| true` keeps set -e from aborting on that.
    shell_out=$(MSWEA_WORKDIR="$shell_work" "$bashy_bin" --bashpp "$adapter" --scenario "$scenarios/$name.json" -y --emit-envelope 2>/dev/null || true)
    yaml_out=$(MSWEA_WORKDIR="$yaml_work" /bin/sh "$yaml_entry" --scenario "$scenarios/$name.json" -y --emit-envelope 2>/dev/null || true)
    gold=$(cat "$result_golden/$name.json")
    [ "$shell_out" = "$gold" ] || fail "parity-$name" "shell != golden"
    [ "$shell_out" = "$yaml_out" ] || fail "parity-$name" "shell != yaml"
    pass "parity-$name"
}
parity_case submit-success
parity_case cost-limit
parity_case step-limit
parity_case repeated-format-error
parity_case format-error-recovers

# --- 3. INTERACTION + INTERRUPTION (real pty / real signal) -------------
for case in pty-confirm pty-human signal; do
    "$python_bin" "$fixtures_dir/interactive_check.py" "$case" || fail "interactive-$case" "see output"
    pass "interactive-$case"
done

# --- 4. LIVE TRANSPORT (loopback fake provider) -------------------------
"$python_bin" "$fixtures_dir/live_check.py" || fail live-transport "see output"
pass live-transport

# --- 5. FAIL-CLOSED + VALIDATION ----------------------------------------
cli() { status=0; "$bashy_bin" --bashpp "$adapter" "$@" >"$scratch/out" 2>"$scratch/err" || status=$?; }

# confirm mode with piped newlines must NOT be treated as approval.
status=0
printf '\n\n\n' | "$bashy_bin" --bashpp "$adapter" --scenario "$scenarios/submit-success.json" --mode confirm >/dev/null 2>"$scratch/err" || status=$?
[ "$status" -eq 4 ] || fail failclosed-confirm "status $status want 4"
grep -q 'fail-closed' "$scratch/err" || fail failclosed-confirm "no fail-closed message"
pass failclosed-confirm-piped

cli -t x --mode human --model-class replay --replay "$scenarios/noop-steps.json" </dev/null
want_status failclosed-human 4; pass failclosed-human

cli --model-class replay --replay "$scenarios/noop-steps.json" -y </dev/null
want_status failclosed-notask 4; pass failclosed-notask

cli -t x -l nan -y --model-class replay --replay "$scenarios/noop-steps.json" </dev/null
want_status budget-nan 3; pass budget-nan

cli -t x -l -5 -y --model-class replay --replay "$scenarios/noop-steps.json" </dev/null
want_status budget-negative 3; pass budget-negative

cli -t x --model-class bogus -y </dev/null
want_status unsupported-model-class 2; pass unsupported-model-class

cli -t x --environment-class docker -y </dev/null
want_status unsupported-env-class 2; pass unsupported-env-class

printf '{not json' > "$scratch/bad.json"
cli -t x -c "$scratch/bad.json" -y --model-class replay --replay "$scenarios/noop-steps.json" </dev/null
want_status malformed-config 3; pass malformed-config

# trajectory persisted on a NON-submit (failed) exit
cli --scenario "$scenarios/cost-limit.json" -y -o "$scratch/traj.json" </dev/null
want_status traj-on-failure 1
"$python_bin" - "$scratch/traj.json" <<'PY' || fail traj-on-failure "no LimitsExceeded in trajectory"
import json, sys
d = json.load(open(sys.argv[1]))
assert d["info"]["exit_status"] == "LimitsExceeded", d["info"]["exit_status"]
PY
pass traj-on-failure

# --- 6. UNIT TESTS ------------------------------------------------------
( cd "$harness_dir" && "$python_bin" -m unittest discover -s tests -q ) >"$scratch/out" 2>&1 || { cat "$scratch/out" >&2; fail unit-tests "see output"; }
pass unit-tests

printf 'PASS %s mini-swe-agent fixtures (both entrypoints run the local loop; live uses a loopback provider; interaction/interruption use a real pty and a real signal)\n' "$passed"
keep=0
