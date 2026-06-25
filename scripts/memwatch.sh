#!/bin/sh
# memwatch.sh <pgid> <cap_kb>
#
# Poll the total RSS of every process in process-group <pgid> and SIGKILL the
# whole group if it exceeds <cap_kb> kilobytes. This is the macOS-safe memory
# guard for the bash 5.3 test harness: `ulimit -v` (RLIMIT_AS) is NOT enforced
# on Darwin, so a runaway fixture (e.g. an unbounded-allocation infinite loop
# like intl/unicode1.sub's printf '\U' under a multibyte locale) can balloon to
# 100+ GB and wedge the machine before the per-test wall-clock timeout fires.
# This watchdog catches the balloon early and turns it into a graceful fixture
# failure instead of an OOM. Exits when the group is gone or after a kill.
pgid="$1"
cap="${2:-4194304}"   # default 4 GB
[ -n "$pgid" ] || exit 0
while kill -0 -"$pgid" 2>/dev/null; do
	total=$(ps ax -o pgid=,rss= 2>/dev/null | awk -v g="$pgid" '$1==g{s+=$2} END{print s+0}')
	if [ "$total" -gt "$cap" ] 2>/dev/null; then
		kill -KILL -- -"$pgid" 2>/dev/null
		exit 0
	fi
	sleep 0.5
done
