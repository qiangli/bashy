#!/bin/sh
# script/bashy-release-qa-lane.sh — umbrella release QA lane over the Bash# tour.
#
# Runs the tour gate (check.sh) against one bashy binary, enforces the
# xfail-card rule on cases.tsv, and prints a summary that always carries
# the known-failing and skipped counts:
#
#   qa-lane: 57 passed, 0 failed, 3 known-failing, 0 skipped (os=linux)
#
# The xfail-card rule keeps known failures from becoming permanent: every
# `os=card` / `ci:os=card` marker in cases.tsv must reference a sprint
# story id that carries a due date — a `todo:<id>` card resolving to a
# story file with `sprint:` and `due:` frontmatter whose due date is a
# real calendar date and not past. A marker without a storied, dated card
# fails this lane, even when the tour itself passes. (An unexpected pass
# where a marker applies already fails inside check.sh, so a marker is
# removed in the same delivery once its case passes.)
#
# Usage:
#   script/bashy-release-qa-lane.sh --tour-dir DIR [--binary PATH | --build]
#       [--sh-root DIR] [--todo-dir DIR]... [--check-only]
#
#   --tour-dir DIR   Bash# tour checkout holding check.sh + cases.tsv.
#                    TOUR_DIR env or this flag; fails closed when absent.
#   --binary PATH    bashy binary under test. Default: BASHY_UNDER_TEST,
#                    else the `bashy` on PATH. --build builds a scratch
#                    binary from this checkout instead (needs go; honor
#                    GOOS/GOARCH in the environment for another OS).
#   --sh-root DIR    BASHSHARP_SH_ROOT for the go/transpile-lowered case.
#                    Default: resolve from this checkout's go.mod pin via
#                    `go mod download`. When unresolvable the tour reports
#                    the case SKIPped and this lane reports the skip count.
#   --todo-dir DIR   extra story register searched for `todo:` cards
#                    (repeatable). Always searched: TOUR_DIR/docs/todo and
#                    this checkout's docs/todo.
#   --check-only     run only the xfail-card rule, not the tour.
#
# check.sh honors TOUR_FILTER (run a subset, e.g. TOUR_FILTER=fleet/) and
# BASHSHARP_SH_ROOT from the environment; both pass straight through.
#
# A native run on another OS is the same command copied there with a tour
# checkout and a native binary — no hostnames, credentials, or site-local
# paths are hardcoded here; the caller supplies them.
set -u

lane=qa-lane
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fail() { echo "$lane: FAIL $*" >&2; exit 1; }
note() { echo "$lane: $*"; }

TOUR_DIR=${TOUR_DIR:-}
BINARY=${BASHY_UNDER_TEST:-}
BUILD=0
SH_ROOT=${BASHSHARP_SH_ROOT:-}
SH_ROOT_EXPLICIT=0
[ -n "$SH_ROOT" ] && SH_ROOT_EXPLICIT=1
TODO_DIRS=""
CHECK_ONLY=0

while [ $# -gt 0 ]; do
    case "$1" in
    --tour-dir) TOUR_DIR=${2:?--tour-dir needs a directory}; shift 2 ;;
    --binary) BINARY=${2:?--binary needs a path}; shift 2 ;;
    --build) BUILD=1; shift ;;
    --sh-root) SH_ROOT=${2:?--sh-root needs a directory}; SH_ROOT_EXPLICIT=1; shift 2 ;;
    --todo-dir) TODO_DIRS="$TODO_DIRS${2:?--todo-dir needs a directory}:"; shift 2 ;;
    --check-only) CHECK_ONLY=1; shift ;;
    -h|--help) sed -n '2,/^set -u/p' "$0" | sed 's/^# //; s/^#$//'; exit 0 ;;
    *) fail "unknown argument: $1 (see --help)" ;;
    esac
done

[ -n "$TOUR_DIR" ] || fail "no tour checkout: pass --tour-dir DIR or set TOUR_DIR"
[ -f "$TOUR_DIR/check.sh" ] || fail "no check.sh in tour dir: $TOUR_DIR"
[ -f "$TOUR_DIR/cases.tsv" ] || fail "no cases.tsv in tour dir: $TOUR_DIR"

# --- story register -------------------------------------------------------
# `todo:<id>` cards resolve against these registers, first hit wins.
TODO_SEARCH="$TOUR_DIR/docs/todo:$root/docs/todo:$TODO_DIRS"

find_story() { # sid -> prints the one story file, or nothing
    sid=$1; hits=""; n=0
    old_ifs=$IFS; IFS=:
    for d in $TODO_SEARCH; do
        [ -n "$d" ] || continue
        for f in "$d/$sid"*.md; do
            [ -f "$f" ] || continue
            hits="$hits$f
"; n=$((n + 1))
        done
    done
    IFS=$old_ifs
    [ "$n" -eq 1 ] && printf '%s' "$hits"
    [ "$n" -le 1 ]
}

story_field() { # file field -> prints first value
    sed -n "s/^$2: \([^[:space:]].*\)/\1/p" "$1" | head -1
}

valid_date() { # YYYY-MM-DD -> 0 iff a real calendar date
    case "$1" in ????-??-??) ;; *) return 1 ;; esac
    y=${1%%-*}; rest=${1#*-}; m=${rest%%-*}; d=${rest#*-}
    case "$y$m$d" in *[!0-9]*) return 1 ;; esac
    m=${m#0}; d=${d#0}; [ -z "$m" ] && m=0; [ -z "$d" ] && d=0
    [ "$m" -ge 1 ] && [ "$m" -le 12 ] || return 1
    case $m in
    1|3|5|7|8|10|12) max=31 ;;
    4|6|9|11) max=30 ;;
    2) max=28
       if [ $((y % 4)) -eq 0 ] && { [ $((y % 100)) -ne 0 ] || [ $((y % 400)) -eq 0 ]; }; then max=29; fi ;;
    esac
    [ "$d" -ge 1 ] && [ "$d" -le "$max" ]
}

past() { # YYYY-MM-DD YYYY-MM-DD(today) -> 0 iff first is before today
    [ "$1" = "$2" ] && return 1
    first=$(printf '%s\n%s\n' "$1" "$2" | sort | head -1)
    [ "$first" = "$1" ]
}

check_cards() { # cases.tsv -> 0 iff every xfail marker is storied + dated
    cases=$1; bad=0; nmarkers=0
    today=$(date +%F) || fail "date +%F failed"
    TAB=$(printf '\t')
    while IFS="$TAB" read -r id mode file needs rc xfail || [ -n "${id:-}" ]; do
        case "${id:-}" in ''|'#'*) continue ;; esac
        [ -n "${xfail:-}" ] || continue
        for spec in $(printf '%s' "$xfail" | tr ';' ' '); do
            nmarkers=$((nmarkers + 1))
            case "$spec" in ci:*=*) rest=${spec#ci:} ;; *=*) rest=$spec ;;
            *) echo "$lane: xfail $id '$spec': malformed (want [ci:]os=card)" >&2; bad=$((bad + 1)); continue ;; esac
            os=${rest%%=*}; card=${rest#*=}
            case "$os" in darwin|linux|windows) ;; *)
                echo "$lane: xfail $id '$spec': unknown os '$os'" >&2; bad=$((bad + 1)); continue ;; esac
            case "$card" in todo:*) sid=${card#todo:} ;; *)
                echo "$lane: xfail $id '$spec': card must be todo:<story-id>" >&2; bad=$((bad + 1)); continue ;; esac
            case "$sid" in *[!0-9a-f]*|'') why="story id is not hex" ;;
                *) [ "${#sid}" -ge 8 ] && why="" || why="story id too short" ;;
            esac
            if [ -n "$why" ]; then
                echo "$lane: xfail $id '$spec': $why: '$sid'" >&2; bad=$((bad + 1)); continue
            fi
            story=$(find_story "$sid") || {
                echo "$lane: xfail $id '$spec': ambiguous story id (several files match)" >&2
                bad=$((bad + 1)); continue
            }
            [ -n "$story" ] || {
                echo "$lane: xfail $id '$spec': no such story (todo:$sid not in the register)" >&2
                bad=$((bad + 1)); continue
            }
            sprint=$(story_field "$story" sprint)
            [ -n "$sprint" ] || {
                echo "$lane: xfail $id '$spec': story $story is on no sprint" >&2
                bad=$((bad + 1)); continue
            }
            due=$(story_field "$story" due)
            [ -n "$due" ] || {
                echo "$lane: xfail $id '$spec': story $story carries no due date" >&2
                bad=$((bad + 1)); continue
            }
            day=$(printf '%s' "$due" | cut -c1-10)
            valid_date "$day" || {
                echo "$lane: xfail $id '$spec': story due date is not a real date: '$due'" >&2
                bad=$((bad + 1)); continue
            }
            past "$day" "$today" && {
                echo "$lane: xfail $id '$spec': story due date $day is past (known failure outlived its story)" >&2
                bad=$((bad + 1)); continue
            }
        done
    done < "$cases"
    note "$nmarkers xfail markers checked against storied, dated cards"
    [ "$bad" -eq 0 ]
}

cards_ok=0
check_cards "$TOUR_DIR/cases.tsv" || cards_ok=1

if [ "$CHECK_ONLY" -eq 1 ]; then
    [ "$cards_ok" -eq 0 ] || fail "$lane: xfail-card rule failed"
    note "xfail-card rule passed"
    exit 0
fi

# --- binary under test ----------------------------------------------------
if [ -z "$BINARY" ]; then
    if [ "$BUILD" -eq 1 ]; then
        command -v go >/dev/null 2>&1 || fail "go not on PATH for --build"
        tmp_bin=$(mktemp -d)/bashy
        (cd "$root" && go build -o "$tmp_bin" ./cmd/bashy) || fail "building ./cmd/bashy failed"
        BINARY=$tmp_bin
    elif command -v bashy >/dev/null 2>&1; then
        BINARY=$(command -v bashy)
    else
        fail "no binary: pass --binary PATH, set BASHY_UNDER_TEST, put bashy on PATH, or use --build"
    fi
fi
[ -x "$BINARY" ] || [ -f "$BINARY" ] || fail "binary is not executable: $BINARY"

# --- lowered-Go source for the go/transpile-lowered case ------------------
if [ -z "$SH_ROOT" ] && command -v go >/dev/null 2>&1; then
    SH_ROOT=$(cd "$root" && go mod download -json github.com/qiangli/sh/v3 2>/dev/null |
        sed -n 's/^ *"Dir": "\(.*\)",\{0,1\}$/\1/p' | head -1)
fi
if [ -n "$SH_ROOT" ]; then
    if [ -d "$SH_ROOT/lower/shellrt" ]; then
        export BASHSHARP_SH_ROOT=$SH_ROOT
    elif [ "$SH_ROOT_EXPLICIT" -eq 1 ]; then
        fail "sh root has no lower/shellrt: $SH_ROOT"
    else
        note "ignoring auto-resolved sh root without lower/shellrt: $SH_ROOT"
        SH_ROOT=""
    fi
else
    note "no sh source resolved (tour reports go/transpile-lowered SKIPped)"
fi

# --- the tour -------------------------------------------------------------
osname=$(uname -s 2>/dev/null | tr 'A-Z' 'a-z'); case "$osname" in linux|darwin) ;; *) osname=windows ;; esac
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT HUP INT TERM
sh "$TOUR_DIR/check.sh" "$BINARY" >"$tmp/tour.log" 2>&1; tour_rc=$?
cat "$tmp/tour.log"
summary=$(grep '^tour: [0-9][0-9]* passed, [0-9][0-9]* failed, [0-9][0-9]* skipped, [0-9][0-9]* known-failing' "$tmp/tour.log" | tail -1)
[ -n "$summary" ] || fail "tour printed no countable summary (known-failing/skipped counts missing)"
pass=$(printf '%s' "$summary" | sed 's/^tour: \([0-9][0-9]*\) passed, .*/\1/')
nfailed=$(printf '%s' "$summary" | sed 's/.* \([0-9][0-9]*\) failed, .*/\1/')
nskip=$(printf '%s' "$summary" | sed 's/.* \([0-9][0-9]*\) skipped, .*/\1/')
nxfail=$(printf '%s' "$summary" | sed 's/.* \([0-9][0-9]*\) known-failing.*/\1/')
note "$pass passed, $nfailed failed, $nxfail known-failing, $nskip skipped (os=$osname)"

[ "$tour_rc" -eq 0 ] || fail "tour gate failed (exit $tour_rc)"
[ "$cards_ok" -eq 0 ] || fail "xfail-card rule failed"
note "release QA passed"
