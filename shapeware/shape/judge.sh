#!/bin/sh
set -eu

mode=$1
bin=$2
here=$(pwd -P)
contract=$here/CONTRACT.md
scratch=$(mktemp -d /tmp/shapeware-shape.XXXXXXXX)
trap 'rm -rf "$scratch"' EXIT
cp -R examples "$scratch/"

# Wishes track their work in the current repo when run from a checkout. Run
# outside it, and give conform's child processes a synthetic owner and no
# inherited agent session or configuration overrides.
run_bashy() {
    env -i PATH="$bin:$PATH" HOME="$scratch" TMPDIR="$scratch" \
        USER=shape-owner LOGNAME=shape-owner BASHY_PRINCIPAL=shape-owner \
        "$bin/bashy" "$@"
}

case "$mode" in
    test)
        run_bashy conform run "$contract" --path "$bin" --dir "$scratch" --fresh-home
        ;;
    cert)
        reference="\"$bin/bashy\" shape"
        report=evidence/shape-cert.json
        mkdir -p evidence
        run_bashy conform cert --contract "$contract" --reference-cmd "$reference" \
            --candidate-cmd "${3:-$reference}" --bin shape --runs 3 \
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
            > evidence/shape-mutate.log 2>&1 || result=$?
        cat evidence/shape-mutate.log
        exit "$result"
        ;;
    *)
        printf 'unknown shape judge mode: %s\n' "$mode" >&2
        exit 2
        ;;
esac
