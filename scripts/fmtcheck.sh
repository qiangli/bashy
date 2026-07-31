#!/bin/sh
# fmtcheck — the formatting gate. REPORTS, NEVER REWRITES.
#
# THE ONE implementation; every caller delegates here so the scope can never
# drift between them:
#   - .github/workflows/test.yml (the ubuntu leg)
#   - dag.md's `fmtcheck` task (manual: `bashy dag fmtcheck`)
#
# # Why this fails instead of fixing
#
# The obvious version of this gate runs `gofmt -w` and commits the result. Do
# not write that version. gofmt does not only move whitespace: on doc comments
# it applies godoc's legacy typographic substitution, turning a pair of ASCII
# single-quotes into a curly closing quote.
#
# That is a silent content change, and it has already bitten THIS repo. The doc
# comment on sortTrapListings in tools/bash53suite/main.go cites the exact
# regression the bash-5.3 baseline once carried — a trap listing whose argument
# is an empty string, written with that very pair of quotes. Auto-formatting
# rewrote it into a curly quote, leaving a signal-fidelity harness documenting
# shell syntax that does not exist. Nothing would have reported it: the diff
# looks like formatting, and formatting diffs are what reviewers skim.
#
# So the rule is: a human decides. When a file legitimately cannot take gofmt's
# output, the fix is to restructure it — move the literal into an indented
# doc-comment code block, where the characters survive verbatim (that file is
# the worked example) — not to accept a rewrite of what the code says.
#
# # Scope
#
# Tracked .go files only, so build output and scratch files never fail the gate.
set -eu

root="$(cd "$(dirname "$0")/.." && pwd -P)"
cd "$root"

files="$(git ls-files '*.go')"
if [ -z "$files" ]; then
	echo "fmtcheck: no tracked Go files"
	exit 0
fi

# shellcheck disable=SC2086
unformatted="$(gofmt -l $files)"
if [ -z "$unformatted" ]; then
	echo "fmtcheck: PASS ($(echo "$files" | wc -l | tr -d ' ') files)"
	exit 0
fi

echo "fmtcheck: FAIL — these files are not gofmt-clean:" >&2
echo "$unformatted" | sed 's/^/  /' >&2
echo >&2
echo "diff:" >&2
# shellcheck disable=SC2086
gofmt -d $unformatted >&2
echo >&2
echo "Fix with: gofmt -w <file>" >&2
echo >&2
echo "READ THE DIFF FIRST. gofmt rewrites doc-comment text as well as" >&2
echo "whitespace — notably a pair of ASCII single-quotes becomes a curly" >&2
echo "closing quote. If a file quotes shell or Go syntax in a comment, move" >&2
echo "the literal into an indented code block rather than accepting the" >&2
echo "rewrite; see the header of this script." >&2
exit 1
