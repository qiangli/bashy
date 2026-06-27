#!/usr/bin/env bash
# dash-posix-suite.sh — run whatever runnable shell code dash ships, with bashy's
# pure-Go `sh` as the shell-under-test, reporting per-shell pass rates vs dash
# and bash as the reference oracles.
#
# ── HONEST SCOPE NOTE (read this before expecting a big number) ──────────────
# dash (the Debian Almquist shell, https://git.kernel.org/pub/scm/utils/dash)
# ships NO standalone POSIX conformance / regression test suite. Its source tree
# is just the C interpreter — there is no `tests/` dir, no `.tst`/`.exp`
# corpus, no harness. dash's real value in THIS project is as an *oracle*, and
# it is already exercised in that role by:
#     scripts/posix-diff.sh        (5-oracle same-env differential; dash = anchor)
#     scripts/multishell-diff.sh   (10-shell panel; dash on both panels)
#     scripts/oils-diff.sh         (Oils case-code through the live differential)
# So this harness is intentionally THIN. It does not fabricate a suite that does
# not exist. The only runnable shell code dash ships is `src/funcs/*` — eight
# BSD-licensed example "loadable function" scripts (cmv dirs kill login newgrp
# popd pushd suspend), the classic ash/dash function library. They are pure
# function *definitions* (no top-level side effects), so loading one under a
# shell in POSIX mode is a real, apples-to-apples check: "does this shell accept
# and load dash's own shipped POSIX shell code?" A couple of them lean on the
# ash brace-less function-body extension (`login () exec login "$@"`), which the
# stricter shells reject — that is exactly the kind of signal this measures.
#
# This is an INFORMATIONAL / relative suite: it reports bashy's load rate next
# to dash's and bash's and exits 0 regardless (the aggregator marks it `@dash`).
# It is NOT a 0-fail gate — dash's own extensions mean even bash does not load
# every file.
#
# The dash source is CLONED AT RUNTIME into a gitignored cache (.dash-tests/)
# and is NEVER vendored. The funcs are BSD-licensed; this harness script is
# permissive. Same posture as the yash GPL test-suite cache in
# scripts/yash-posix-suite.sh — model the container plumbing on that script.
#
# Usage: scripts/dash-posix-suite.sh
# Requires: a container runtime (docker / `ycode podman`) + Go + git.
set -u
HERE=$(cd "$(dirname "$0")/.." && pwd)
cd "$HERE" || exit 2
DT="$HERE/.dash-tests"   # gitignored clone cache

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  command -v docker >/dev/null 2>&1 && OCI=docker || { command -v ycode >/dev/null 2>&1 && OCI="ycode podman"; }
fi
[ -n "$OCI" ] || { echo "dash-suite: need docker or ycode podman" >&2; exit 0; }

# Clone dash (shallow) for its src/funcs/ — never committed.
if [ ! -d "$DT/src/funcs" ]; then
  echo "dash-suite: cloning dash (upstream, gitignored cache)…" >&2
  rm -rf "$DT"
  git clone --depth 1 https://git.kernel.org/pub/scm/utils/dash/dash.git "$DT" >&2 \
    || git clone --depth 1 https://github.com/danishprakash/dash.git "$DT" >&2 \
    || { echo "dash-suite: clone failed (network?) — skipping" >&2; exit 0; }
fi
FUNCS_DIR="$DT/src/funcs"
if [ ! -d "$FUNCS_DIR" ]; then
  echo "dash-suite: no src/funcs/ in the dash checkout — nothing runnable, skipping" >&2
  exit 0
fi
NFILES=$(find "$FUNCS_DIR" -maxdepth 1 -type f ! -name '*.[ch]' ! -name '*.in' | wc -l | tr -d ' ')
echo "##### dash runnable-shell-code suite vs bashy + reference shells #####"
echo "# (dash ships NO conformance suite; this loads its $NFILES src/funcs/* example"
echo "#  function scripts as 'sh'. dash's primary role stays ORACLE — see"
echo "#  scripts/posix-diff.sh + scripts/multishell-diff.sh.)"

# Build/reuse a distinct oracle image: exact bash 5.3 + dash + busybox ash.
IMAGE=localhost/posix-shells-dash
if ! $OCI image exists "$IMAGE" 2>/dev/null; then
  echo "dash-suite: building $IMAGE …" >&2
  bd=$(mktemp -d); printf '%b' 'FROM bash:5.3\nRUN apk add --no-cache dash\n' > "$bd/Containerfile"
  $OCI build -q -t "$IMAGE" "$bd" >&2 || { echo "dash-suite: image build failed — skipping" >&2; exit 0; }
  rm -rf "$bd"
fi

# Cross-compile bashy to the container arch (keep under bin/ — macOS /tmp is a
# /private symlink the container runtime refuses to bind-mount).
ARCH=$($OCI run --rm "$IMAGE" uname -m | tr -d '\r')
case "$ARCH" in aarch64|arm64) GOARCH=arm64;; x86_64|amd64) GOARCH=amd64;; *) echo "dash-suite: bad arch $ARCH" >&2; exit 0;; esac
BIN="$HERE/bin/.bashy-linux-dash"
mkdir -p "$(dirname "$BIN")"
echo "dash-suite: building linux/$GOARCH bashy…" >&2
GOOS=linux GOARCH="$GOARCH" go build -o "$BIN" ./cmd/bash || { echo "dash-suite: bashy build failed" >&2; exit 0; }
trap 'rm -f "$BIN"' EXIT

# Run all shells in one container so they share one userland (busybox coreutils,
# same $HOME) — isolates SHELL behavior. For each func file, source it in POSIX
# mode in a fresh subshell; PASS = the shell loads dash's code without error.
# bashy emits the `bashy …` line the aggregator (posix-certdryrun.sh) greps for.
$OCI run --rm -e LANG=C \
  -v "$FUNCS_DIR:/df:ro" -v "$BIN:/bashy:ro" "$IMAGE" busybox ash -c '
  export LANG=C
  # "label=cmd" — cmd is the shell invocation put in POSIX mode.
  SHELLS="bashy=/bashy --posix dash=dash bash53=bash --posix ash=busybox ash"
  FILES=""; for f in /df/*; do
    case "$f" in *.c|*.h|*.1|*.in) continue;; esac
    [ -f "$f" ] && FILES="$FILES $f"
  done
  tot=0; for f in $FILES; do tot=$((tot+1)); done
  # Print one row per shell; bashy first so its line is unambiguous.
  for spec in "bashy=/bashy --posix" "dash=dash" "bash53=bash --posix" "ash=busybox ash"; do
    label=${spec%%=*}; cmd=${spec#*=}
    bin=${cmd%% *}
    case "$bin" in /*) [ -x "$bin" ] || { printf "  %-7s (not found)\n" "$label"; continue; };;
      *) command -v "$bin" >/dev/null 2>&1 || { printf "  %-7s (not found)\n" "$label"; continue; };; esac
    ok=0; detail=""
    for f in $FILES; do
      b=$(basename "$f")
      if echo "" | timeout 8 $cmd -c ". $f" >/dev/null 2>&1; then
        ok=$((ok+1)); detail="$detail +$b"
      else
        detail="$detail -$b"
      fi
    done
    pct="n/a"; [ "$tot" -gt 0 ] && pct="$((ok*100/tot))%"
    printf "  %-7s loaded=%d/%d (%s)\n" "$label" "$ok" "$tot" "$pct"
    printf "          %s\n" "${detail# }"
  done
  echo "  (+ = loaded clean, - = rejected; rejects are dash ash-extension funcs"
  echo "   the stricter shells decline — expected, not a bashy bug)"
' 2>&1 | grep -vE 'WARN|sysctl'

echo "=== dash suite: informational (relative load rate; dash stays an oracle) ==="
exit 0
