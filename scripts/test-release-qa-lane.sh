#!/bin/sh
# scripts/test-release-qa-lane.sh — focused test for script/bashy-release-qa-lane.sh.
#
# Hermetic: every tour checkout, cases.tsv, story register, and check.sh
# stub is built under a temp dir. No network, no hostnames.
set -u

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
lane=$script_dir/../script/bashy-release-qa-lane.sh
tmp=${TMPDIR:-/tmp}/test-qa-lane.$$
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir -p "$tmp/todo" "$tmp/tour"
printf '#!/bin/sh\nexit 0\n' >"$tmp/fake-bashy"

story() { # id body... -> writes todo/<id>-story.md
    id=$1; shift
    { printf -- '---\nid: %s\nkind: task\ntitle: probe\nseq: 1\nstatus: todo\n' "$id"; printf '%s\n' "$@"; printf -- '---\n'; } >"$tmp/todo/$id-story.md"
}
story deadbeef12345678 'sprint: 517' 'due: 2099-01-15T00:00:00Z'
story feedface12345678 'sprint: 517'
story cafef00d12345678 'due: 2099-01-15T00:00:00Z'
story 0badc0de12345678 'sprint: 517' 'due: 2001-02-03T00:00:00Z'

# A stub tour: canned transcript lines plus a check.sh-style summary.
# STUB_SUMMARY and STUB_RC tune it per case.
mk_tour() { # cases-body...
    rm -f "$tmp/tour/cases.tsv" "$tmp/tour/check.sh"
    printf '# id\tmode\tfile\tneeds\trc\txfail\n%s\n' "$1" >"$tmp/tour/cases.tsv"
    cat >"$tmp/tour/check.sh" <<'EOF'
#!/bin/sh
printf '%s\n' "$STUB_LINES"
printf 'tour: %s\n' "$STUB_SUMMARY"
exit "${STUB_RC:-0}"
EOF
}
run_lane() { # args... -> runs the lane against the fixture tour+register
    STUB_LINES=${STUB_LINES:-} STUB_SUMMARY=${STUB_SUMMARY:-} STUB_RC=${STUB_RC:-0} \
        sh "$lane" --tour-dir "$tmp/tour" --todo-dir "$tmp/todo" "$@"
}
expect_ok() {
    out=$("$@" 2>&1) || { echo "expected success: $*"; printf '%s\n' "$out"; exit 1; }
    printf '%s\n' "$out"
}
expect_fail() {
    if out=$("$@" 2>&1); then echo "expected failure: $*"; printf '%s\n' "$out"; exit 1; fi
    printf '%s\n' "$out"
}

TAB=$(printf '\t')

# 1. No markers: tour summary counts are re-printed by the lane.
STUB_LINES='tour: PASS a' STUB_SUMMARY='3 passed, 0 failed, 1 skipped, 2 known-failing (v)' STUB_RC=0 \
    mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0"
out=$(expect_ok run_lane --binary "$tmp/fake-bashy")
printf '%s\n' "$out" | grep -q 'qa-lane: 3 passed, 0 failed, 2 known-failing, 1 skipped' ||
    { echo "lane summary must carry known-failing and skipped counts"; printf '%s\n' "$out"; exit 1; }

# 2. Storied + dated markers pass --check-only.
mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0${TAB}windows=todo:deadbeef12345678;ci:darwin=todo:deadbeef12345678"
expect_ok run_lane --binary "$tmp/fake-bashy" --check-only >/dev/null

# 3. Each undated/unstoried shape fails --check-only.
mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0${TAB}windows=todo:79d2994c"
expect_fail run_lane --binary "$tmp/fake-bashy" --check-only >/dev/null
mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0${TAB}windows=todo:feedface12345678"
out=$(expect_fail run_lane --binary "$tmp/fake-bashy" --check-only); printf '%s\n' "$out" | grep -q 'no due date' || exit 1
mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0${TAB}windows=todo:cafef00d12345678"
out=$(expect_fail run_lane --binary "$tmp/fake-bashy" --check-only); printf '%s\n' "$out" | grep -q 'on no sprint' || exit 1
mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0${TAB}windows=todo:0badc0de12345678"
out=$(expect_fail run_lane --binary "$tmp/fake-bashy" --check-only); printf '%s\n' "$out" | grep -q 'is past' || exit 1
mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0${TAB}plan9=todo:deadbeef12345678"
expect_fail run_lane --binary "$tmp/fake-bashy" --check-only >/dev/null
mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0${TAB}windows=web:1234"
expect_fail run_lane --binary "$tmp/fake-bashy" --check-only >/dev/null

# 4. Ambiguous story id fails closed.
story deadbeef12345678b 'sprint: 517' 'due: 2099-01-15T00:00:00Z'
story deadbeef12345678c 'sprint: 517' 'due: 2099-01-15T00:00:00Z'
mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0${TAB}windows=todo:deadbeef12345678"
expect_fail run_lane --binary "$tmp/fake-bashy" --check-only >/dev/null
rm "$tmp/todo/deadbeef12345678b-story.md" "$tmp/todo/deadbeef12345678c-story.md"

# 5. A tour failure fails the lane even with clean cards; the lane still
#    prints its counts summary.
STUB_LINES='tour: FAIL a' STUB_SUMMARY='1 passed, 1 failed, 0 skipped, 0 known-failing (v)' STUB_RC=1 \
    mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0"
out=$(expect_fail run_lane --binary "$tmp/fake-bashy"); printf '%s\n' "$out" | grep -q 'qa-lane: 1 passed, 1 failed' || exit 1

# 6. A tour that prints no countable summary fails the lane closed.
STUB_LINES='tour: PASS a' STUB_SUMMARY='all good (no numbers)' STUB_RC=0 \
    mk_tour "a${TAB}run${TAB}f.bsh${TAB}-${TAB}0"
out=$(expect_fail run_lane --binary "$tmp/fake-bashy"); printf '%s\n' "$out" | grep -q 'no countable summary' || exit 1

echo 'test-release-qa-lane: PASS'
