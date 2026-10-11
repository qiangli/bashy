#!/bin/sh
# Sprint 216 B19 gate (rfcs/0001-agentic-yield.md): replay the agentic-yield
# harness loop on a bashy binary (default: the one on PATH — the INSTALLED
# binary) and diff the combined transcript against the pinned recording.
# Exit 0 = pass.
#
#   test/yield/run.sh [bashy]
#
# The loop this drives is the RFC's §Harness, end to end and for real:
#
#   1  normal    — `clean --what-if` describes every governed operation
#                  (with the high-impact one's token) and performs none.
#   2  handoff   — an unattended `clean` YIELDS: exit 6, the high-impact
#                  operation not performed, every later side effect refused.
#   3  pause+ask — the yield also raised an approval in the shared ask queue.
#                  The driver pauses, takes the token and the approval id OFF
#                  the yield line, and obtains the answer from the human. Here
#                  the human's reply is the RECORDED one in ./answer ("yes",
#                  recorded 2026-09-20); the driver never invents it, and a
#                  missing recording makes THIS script exit 6 in turn.
#   4  replay    — the answer is recorded with `bashy approve allow ID` (which
#                  prints the resume flag), then `clean --confirm=TOKEN:yes`
#                  redeems it: exit 0, converged. The allow is single-use.
#   5  boundary  — an answer for a DIFFERENT token makes no progress: the
#                  same stable token is named again (a harness seeing the
#                  same token twice has its stop condition). So does a
#                  repeat of the spent yes from step 4.
#   6  failure   — `--confirm=TOKEN:no` refuses that operation (126 inside,
#                  body continues, exit 0) and the victim survives.
#   7  boundary  — a malformed --confirm= is a diagnostic (exit 1), not a
#                  guess; nothing in the body runs.
#   8  handoff   — under BASHY_AGENTIC=1 the same yield is a weavecli JSON
#                  envelope (error.code "input_required"), still exit 6.
#   9  identity  — Bash OFF: the undecorated body in plain Bash mode sees
#                  --what-if as an ordinary word and rm just runs.
#
# Every path in the fixture is relative and the SUT runs on a scrubbed
# environment, so the transcript — tokens included — is byte-stable. The
# approval ask-queue lives in the work dir and its random ids are scrubbed.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
bashy=${1:-bashy}
case $bashy in
*/*) bashy=$(cd "$(dirname "$bashy")" && pwd)/$(basename "$bashy") ;;
*) bashy=$(command -v "$bashy") ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# fresh NAME — a workdir with the fixture's marker file and victim dir.
fresh() {
	rm -rf "$work/$1"
	mkdir -p "$work/$1/victim"
	printf 'marker\n' >"$work/$1/marker"
}

# sut DIR [ENV=V ...] -- ARG... — one hermetic SUT run in DIR: scrubbed
# environment, combined output, and the exit status as an `exit=` line.
sut() {
	dir=$1
	shift
	envs=
	while [ "$1" != "--" ]; do
		envs="$envs $1"
		shift
	done
	shift
	st=0
	(cd "$work/$dir" && /usr/bin/env -i HOME="$HOME" PATH=/usr/bin:/bin \
		BASHY_ASK_DIR="$work/ask" BASHY_SESSION_CLAIM=yield-fixture \
		$envs "$bashy" "$@" 2>&1) || st=$?
	echo "exit=$st"
}

state() {
	s="state:"
	for f in victim after; do
		if [ -e "$work/$1/$f" ]; then s="$s $f=present"; else s="$s $f=absent"; fi
	done
	echo "$s"
}

loop() {
	echo "== 1 what-if: describe every governed operation, perform none =="
	fresh w
	sut w BASHY_ASK_HANDLER=recorded -- --bashsharp "$here/clean.bsh" --what-if
	state w

	echo "== 2 unattended run: the yield — exit 6, nothing destroyed =="
	yield=$(sut w BASHY_ASK_HANDLER=recorded -- --bashsharp "$here/clean.bsh")
	printf '%s\n' "$yield"
	state w

	echo "== 3 the harness pauses and asks the human =="
	tok=$(printf '%s\n' "$yield" |
		sed -n 's/.*resume with clean --confirm=\([0-9a-f]\{8\}\):yes.*/\1/p')
	aid=$(printf '%s\n' "$yield" |
		sed -n 's/.*bashy approve allow \([0-9a-f]\{8\}\).*/\1/p')
	echo "resume form: clean --confirm=$tok:yes|no"
	if [ ! -f "$here/answer" ]; then
		echo "input required: no recorded human answer in $here/answer" >&2
		exit 6
	fi
	answer=$(cat "$here/answer")
	echo "human answered: $answer"

	echo "== 4 record the answer, then replay with it =="
	case $answer in
	yes) verb=allow ;;
	*) verb=deny ;;
	esac
	sut w -- approve "$verb" "$aid"
	sut w BASHY_ASK_HANDLER=recorded -- --bashsharp "$here/clean.bsh" "--confirm=$tok:$answer"
	state w

	echo "== 5 an answer for another operation, or a spent one, makes no progress =="
	fresh w
	sut w BASHY_ASK_HANDLER=recorded -- --bashsharp "$here/clean.bsh" --confirm=deadbeef:yes
	state w
	sut w BASHY_ASK_HANDLER=recorded -- --bashsharp "$here/clean.bsh" "--confirm=$tok:yes"
	state w

	echo "== 6 the human said no: refused, body continues =="
	fresh w
	sut w BASHY_ASK_HANDLER=recorded -- --bashsharp "$here/clean.bsh" "--confirm=$tok:no"
	state w

	echo "== 7 a malformed answer list is a diagnostic, never a guess =="
	fresh w
	sut w BASHY_ASK_HANDLER=recorded -- --bashsharp "$here/clean.bsh" --confirm=nonsense
	state w

	echo "== 8 the same yield under BASHY_AGENTIC=1 is an envelope =="
	fresh w
	sut w BASHY_AGENTIC=1 -- --bashsharp "$here/clean.bsh"
	state w

	echo "== 9 Bash OFF is identity: --what-if is an ordinary word =="
	fresh w
	sut w BASHY_ASK_HANDLER=recorded -- \
		-c 'clean() { rm -rf victim; echo "cleaned ${1-}"; }; clean --what-if'
	state w
}

out=$(loop | sed \
	-e 's/approval [0-9a-f]\{8\}:/approval ID:/' \
	-e 's/approve allow [0-9a-f]\{8\}/approve allow ID/g' \
	-e 's/approve deny [0-9a-f]\{8\}/approve deny ID/g' \
	-e 's/^\(allow\|deny\) [0-9a-f]\{8\} (token/\1 ID (token/')
if [ "$out" = "$(cat "$here/transcript.expected")" ]; then
	echo "yield: PASS ($bashy)"
else
	echo "yield: FAIL ($bashy)" >&2
	printf '%s\n' "$out" | diff -u "$here/transcript.expected" - >&2 || true
	exit 1
fi
