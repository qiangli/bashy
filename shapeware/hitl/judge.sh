#!/bin/sh
set -eu

mode=$1
bin=$2
here=$(pwd -P)
contract=$here/CONTRACT.md
scratch=$(mktemp -d /tmp/shapeware-hitl.XXXXXXXX)
trap 'rm -rf "$scratch"' EXIT

# The contract's callers are stated by the launcher, never inherited: conform's
# children get no agent session and no configuration overrides from this
# process. The Go toolchain and its caches are the venue's (SKILL.md,
# "Operating the bashy reference"): the mb and inbox adapters compile on first
# use, and a fresh HOME must download nothing.
gobin=$(dirname "$(command -v go)")
run_bashy() {
    env -i PATH="$bin:$gobin:$PATH" HOME="$scratch" TMPDIR="$scratch" \
        USER=tester LOGNAME=tester BASHY_HINTS=off \
        GOMODCACHE="$(go env GOMODCACHE)" GOCACHE="$(go env GOCACHE)" \
        GOTOOLCHAIN=local \
        "$bin/bashy" "$@"
}

case "$mode" in
    test)
        run_bashy conform run "$contract" --path "$bin" --dir "$scratch" --fresh-home
        ;;
    cert)
        reference="\"$bin/session\""
        report=evidence/hitl-cert.json
        mkdir -p evidence
        run_bashy conform cert --contract "$contract" --reference-cmd "$reference" \
            --candidate-cmd "${3:-$reference}" --bin session --runs 3 \
            --dir "$scratch" --fresh-home --out "$report"
        if ! run_bashy -c 'jq -e ".baseline.noisy == []" "$1" >/dev/null' _ "$report"; then
            printf '%s\n' "Noisy reference assertions. See $report." >&2
            exit 1
        fi
        ;;
    mutate)
        mkdir -p evidence
        result=0
        run_bashy conform mutate "$contract" --path "$bin" --dir "$scratch" --fresh-home \
            > evidence/hitl-mutate.log 2>&1 || result=$?
        cat evidence/hitl-mutate.log
        exit "$result"
        ;;
    *)
        printf 'unknown hitl judge mode: %s\n' "$mode" >&2
        exit 2
        ;;
esac
