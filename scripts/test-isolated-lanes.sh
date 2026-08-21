#!/bin/sh
set -eu
repo=$(CDPATH= cd -P "$(dirname "$0")/.." && pwd)
. "$repo/scripts/test-lane-id.sh"

fail() { echo "test-isolated-lanes: $*" >&2; exit 1; }
a=$(BASHY_TEST_LANE=agent-one bashy_test_lane "$repo")
b=$(BASHY_TEST_LANE=agent-two bashy_test_lane "$repo")
[ "$a" = agent-one ] || fail "explicit lane was not preserved: $a"
[ "$a" != "$b" ] || fail 'distinct agents resolved to one lane'
auto=$(env -u BASHY_TEST_LANE sh -c '. "$1"; bashy_test_lane "$2"' sh \
  "$repo/scripts/test-lane-id.sh" "$repo")
case "$auto" in ws-[0-9]*) ;; *) fail "workspace lane is unstable or unsafe: $auto" ;; esac
grep -q 'bashy-self-$lane' "$repo/scripts/test-self-container.sh" ||
  fail 'self-test container does not use the lane identity'
grep -q 'bashy-bash53-$LANE' "$repo/scripts/test-bash-container.sh" ||
  fail 'Bash 5.3 container does not use the lane identity'
grep -q 'bashy-yash-$LANE' "$repo/scripts/yash-scoreboard.sh" ||
  fail 'Yash container does not use the lane identity'
echo 'test-isolated-lanes: PASS'
