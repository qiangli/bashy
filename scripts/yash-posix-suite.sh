#!/usr/bin/env bash
# yash-posix-suite.sh — run yash's POSIX (-p) conformance suite against bashy +
# the reference shells, reporting per-shell pass rates AND dumping per-testcase
# verdicts (for pairwise-closeness baselines + bash-gap triage).
#
# yash's test suite is GPL. This harness CLONES it at runtime into a gitignored
# cache (.yash-tests/) and NEVER vendors it into the repo — the same posture as
# the bash-5.3 fixture symlink. The harness script itself is permissive.
#
# Scope: only the *-p.tst files (yash's POSIX, shell-agnostic tests — they run
# the testee as `sh`/POSIX mode). The job-control + signal tests (sig*, bg/fg/
# job/kill/wait/testtty/async) need a controlling TTY + yash's checkfg C helper
# and hang headlessly — the same scope as bashy's documented interactive-JC
# limitation — so they are excluded uniformly for ALL shells (apples-to-apples).
#
# The framework is driven under `busybox ash` (consistent, full testcase count;
# bash-as-runner truncates, dash-as-runner trips on $LINENO under set -u).
#
# Usage: scripts/yash-posix-suite.sh [verdict-outdir]
#   No arg: just print per-shell pass rates.
#   With outdir: also write <panel>.<shell>.verdicts (lines "<file> <n> OK|ERROR")
#                for the pairwise/triage analysis.
# Requires: a container runtime (docker / bashy podman) + Go + git.
set -u
HERE=$(cd "$(dirname "$0")/.." && pwd)
cd "$HERE" || exit 2
OUTDIR=${1:-}
YT="$HERE/.yash-tests"   # gitignored clone cache

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif [ -n "${BASHY:-}" ]; then OCI="$BASHY podman"
  elif command -v bashy >/dev/null 2>&1; then OCI="bashy podman"
  fi
fi
[ -n "$OCI" ] || { echo "yash-suite: need docker or bashy podman" >&2; exit 2; }

# Clone yash (shallow) for its tests/ — never committed.
if [ ! -d "$YT/tests" ]; then
  echo "yash-suite: cloning yash (GPL test suite, gitignored cache)…" >&2
  rm -rf "$YT"; git clone --depth 1 https://github.com/magicant/yash.git "$YT" >&2 || { echo "clone failed" >&2; exit 2; }
fi
TESTS_DIR="$YT/tests"

# Build/reuse the two oracle images (same as multishell-diff.sh).
build_image() { # name dockerfile
  $OCI image exists "$1" 2>/dev/null && return 0
  echo "yash-suite: building $1 …" >&2
  bd=$(mktemp -d); printf '%b' "$2" > "$bd/Containerfile"
  $OCI build -q -t "$1" "$bd" >&2 || { echo "image build failed" >&2; exit 2; }
  rm -rf "$bd"
}
build_image localhost/posix-shells-broad $'FROM bash:5.3\nRUN apk add --no-cache dash yash zsh mksh loksh\n'
build_image localhost/posix-shells-deb $'FROM debian:stable-slim\nRUN apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq posh ksh dash zsh mksh busybox >/dev/null 2>&1\n'

# Cross-compile bashy to the container arch.
ARCH=$($OCI run --rm localhost/posix-shells-broad uname -m | tr -d '\r')
case "$ARCH" in aarch64|arm64) GOARCH=arm64;; x86_64|amd64) GOARCH=amd64;; *) echo "bad arch $ARCH" >&2; exit 2;; esac
BIN="$HERE/bin/.bashy-linux-yash-$$"
echo "yash-suite: building linux/$GOARCH bashy…" >&2
GOOS=linux GOARCH="$GOARCH" go build -o "$BIN" ./cmd/bash || exit 2
trap 'rm -f "$BIN"' EXIT

[ -n "$OUTDIR" ] && { mkdir -p "$OUTDIR"; OUTMOUNT="-v $OUTDIR:/out"; } || OUTMOUNT=""

run_panel() { # panel-label image "label=cmd …"
  echo "### Panel: $1 ###"
  $OCI run --rm $OUTMOUNT -e LANG=C -e PANEL="$1" -e SPECS="$3" \
    -v "$TESTS_DIR:/yt:ro" -v "$BIN:/bashy:ro" "$2" busybox ash -c '
    export LANG=C; cp -r /yt /work; cd /work
    TESTS=""; for t in *-p.tst; do
      case "$t" in sig*|bg-p.tst|fg-p.tst|job-p.tst|kill*-p.tst|wait-p.tst|testtty-p.tst|async-p.tst) continue;; esac
      TESTS="$TESTS $t"
    done
    for spec in $SPECS; do
      label=${spec%%=*}; cmd=${spec#*=}
      command -v "$cmd" >/dev/null 2>&1 || { echo "  $label: (not found)"; continue; }
      ok=0; er=0; vf="/out/$PANEL.$label.verdicts"; [ -d /out ] && : > "$vf"
      for t in $TESTS; do
        timeout -s KILL 8 busybox ash run-test.sh "$cmd" "$t" >/dev/null 2>&1
        trs="${t%.tst}.trs"; [ -f "$trs" ] || continue
        o=$(grep -cE "^%%% (OK\[|ERROR\[PASSED_UNEXPECTEDLY\])" "$trs" 2>/dev/null); ok=$((ok + ${o:-0}))
        e=$(grep -c "^%%% ERROR\[FAILED\]" "$trs" 2>/dev/null); er=$((er + ${e:-0}))
        if [ -d /out ]; then
          sed -nE \
            -e "s/^%%% OK\[[^]]*\]: [^:]+:([0-9]+):.*/$t \1 OK/p" \
            -e "s/^%%% ERROR\[PASSED_UNEXPECTEDLY\]: [^:]+:([0-9]+):.*/$t \1 OK/p" \
            -e "s/^%%% ERROR\[FAILED\]: [^:]+:([0-9]+):.*/$t \1 ERROR/p" \
            "$trs" >> "$vf"
          if [ "$label" = bashy ] && [ "${e:-0}" -gt 0 ]; then
            cp "$trs" "/out/$PANEL.$label.$trs"
          fi
        fi
      done
      tot=$((ok+er)); pct="n/a"; [ "$tot" -gt 0 ] && pct="$((ok*100/tot))%"
      printf "  %-8s OK=%-4d ERROR=%-4d -> %s pass (of %d)\n" "$label" "$ok" "$er" "$pct" "$tot"
    done' 2>&1 | grep -vE 'WARN|sysctl'
}

echo "##### yash POSIX (-p) suite vs bashy + reference shells #####"
run_panel alpine localhost/posix-shells-broad "bashy=/bashy bash53=bash dash=dash ash=/bin/ash yash=yash mksh=mksh loksh=ksh zsh=zsh"
run_panel debian localhost/posix-shells-deb "bashy=/bashy bash52=bash dash=dash posh=posh ksh93=ksh mksh=mksh zsh=zsh"
[ -n "$OUTDIR" ] && echo "verdicts written to $OUTDIR/ (<panel>.<shell>.verdicts)"
exit 0
