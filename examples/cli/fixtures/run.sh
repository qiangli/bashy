#!/bin/sh
set -eu

fixtures_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
profile="$fixtures_dir/../ycode/profile.yaml"
adapter="$fixtures_dir/../ycode/main.bsh"
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

scratch=$(mktemp -d "${TMPDIR:-/tmp}/bashy-cli-ycode.XXXXXX")
keep=1
trap 'if [ "$keep" -eq 0 ]; then rm -rf "$scratch"; else printf "comparison evidence: %s\n" "$scratch" >&2; fi' EXIT
trap 'exit 1' HUP INT TERM
export YCODE_BIN
export YCODE_CONFIG=
export XDG_CONFIG_HOME="$scratch/config"
export XDG_DATA_HOME="$scratch/data"
export BASHY_HOME="$scratch/bashy"
export BASHY_KB_DIR="$scratch/kb"
export BASHY_SKILLS_DIR="$scratch/skills"
export BASHY_HINTS=off
export LC_ALL=C
cd "$scratch"
printf '%s\n' 'Offline ycode adapter fixture workspace.' >AGENTS.md

passed=0
compare() {
    label=$1
    expected=$2
    shift 2
    direct_status=0
    "$YCODE_BIN" --file "$profile" "$@" >direct.out 2>direct.err || direct_status=$?
    adapter_status=0
    "$bashy_bin" --bashsharp "$adapter" "$@" >adapter.out 2>adapter.err || adapter_status=$?
    if [ "$direct_status" -ne "$adapter_status" ]; then
        printf '%s: status direct=%s adapter=%s\n' "$label" "$direct_status" "$adapter_status" >&2
        cat direct.err adapter.err >&2
        exit 1
    fi
    case "$expected:$direct_status" in
        success:0) ;;
        failure:0) printf '%s: unexpectedly succeeded\n' "$label" >&2; exit 1 ;;
        failure:*) ;;
        *) printf '%s: unexpectedly failed (%s)\n' "$label" "$direct_status" >&2; cat direct.err >&2; exit 1 ;;
    esac
    cmp direct.out adapter.out || { printf '%s: stdout differs\n' "$label" >&2; exit 1; }
    cmp direct.err adapter.err || { printf '%s: stderr differs\n' "$label" >&2; cat direct.err adapter.err >&2; exit 1; }
    passed=$((passed + 1))
    printf 'PASS %s\n' "$label"
}

compare help success --help
compare nested-help success docs --help
compare version success version
compare validate success validate
compare config-get success config get spec.runtime.defaultAgentRef
compare model-current success model current
compare tools-json success tools --json list
compare docs-list success docs --list
compare docs-catalog success docs catalog --json --task shell
compare features-list success features list
compare explicit-file success --file "$profile" validate
compare joined-file success "-f$profile" validate
compare unknown-flag failure config --not-a-ycode-flag
compare unknown-command failure config not-a-ycode-command
compare missing-key failure config get spec.runtime.noSuchField
compare missing-file failure --file "$scratch/missing.yaml" validate

printf 'PASS %s offline ycode adapter comparisons\n' "$passed"
keep=0
