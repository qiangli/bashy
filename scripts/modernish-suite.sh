#!/usr/bin/env bash
# modernish-suite.sh — run the modernish project's own regression/feature test
# suite with bashy's pure-Go `sh` as the shell-under-test, and report bashy's
# pass profile alongside reference shells (bash, dash, mksh, zsh, yash).
#
# modernish (https://github.com/modernish/modernish) is an MIT-licensed POSIX
# shell library. It ships a self-test ("modernish --test") that can run under
# ANY shell by invoking that shell on its bin/modernish loader:
#       <shell> bin/modernish --test [-q...]
# This is exactly what its own install.sh does before installing
# (install.sh line ~380: `$msh_shell $MSH_PREFIX/bin/modernish --test -eqq`),
# so it works straight from the source tree with no install step. The suite is
# ~389 regression tests of modernish's own functionality, exercised through the
# host shell's parameter expansion / arithmetic / aliases / traps / etc.
#
# This is an INFORMATIONAL / relative suite, not a pass-or-fail gate:
#   * modernish is STRICT about its host shell. It runs a fatal-bug self-test at
#     init (lib/modernish/adj/fatal.sh); a shell that trips it cannot run the
#     suite AT ALL. On the alpine reference set, bash / mksh / zsh initialise
#     and pass ~96-98% of tests, but dash and yash FAIL modernish init outright
#     (modernish does not support them). So "0 fails for everyone" is the wrong
#     measure — what matters is bashy's profile vs. the shells modernish does
#     support.
#   * Therefore this harness ALWAYS exits 0 after reporting. The aggregator
#     (scripts/posix-certdryrun.sh) marks it `@modernish` = INFO and surfaces
#     the `bashy` line rather than gating on exit code.
#
# Like the yash harness, modernish is CLONED at runtime into a gitignored cache
# (.modernish-tests/) and NEVER vendored into the repo. The harness itself is
# plain permissively-licensed bash.
#
# Usage: scripts/modernish-suite.sh
# Env:   OCI="bashy podman"   (override container runtime; auto-detected)
# Requires: a container runtime (docker / bashy podman) + Go + git.
set -u
HERE=$(cd "$(dirname "$0")/.." && pwd)
cd "$HERE" || exit 2
MT="$HERE/.modernish-tests"   # gitignored clone cache

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif [ -n "${BASHY:-}" ]; then OCI="$BASHY podman"
  elif command -v bashy >/dev/null 2>&1; then OCI="bashy podman"
  fi
fi
[ -n "$OCI" ] || { echo "modernish-suite: need docker or bashy podman" >&2; exit 0; }

# Clone modernish (shallow) — never committed.
if [ ! -d "$MT/bin" ] || [ ! -f "$MT/bin/modernish" ]; then
  echo "modernish-suite: cloning modernish (MIT, gitignored cache)…" >&2
  rm -rf "$MT"
  git clone --depth 1 https://github.com/modernish/modernish.git "$MT" >&2 \
    || { echo "modernish-suite: clone failed" >&2; exit 0; }
fi

# Build/reuse the reference-shell image (bash + the shells modernish supports,
# plus dash/yash as 'does-modernish-even-accept-it' reference points).
IMG=localhost/posix-shells-modernish
if ! $OCI image exists "$IMG" 2>/dev/null; then
  echo "modernish-suite: building $IMG …" >&2
  bd=$(mktemp -d)
  printf '%s\n' \
    'FROM bash:5.3' \
    'RUN apk add --no-cache dash mksh zsh yash coreutils grep sed findutils diffutils' \
    > "$bd/Containerfile"
  $OCI build -q -t "$IMG" "$bd" >&2 || { echo "modernish-suite: image build failed" >&2; rm -rf "$bd"; exit 0; }
  rm -rf "$bd"
fi

# Cross-compile bashy's pure-Go bash drop-in to the container arch.
ARCH=$($OCI run --rm "$IMG" uname -m | tr -d '\r')
case "$ARCH" in
  aarch64|arm64) GOARCH=arm64;;
  x86_64|amd64)  GOARCH=amd64;;
  *) echo "modernish-suite: unsupported arch $ARCH" >&2; exit 0;;
esac
BIN="$HERE/bin/.bashy-linux-modernish"
echo "modernish-suite: building linux/$GOARCH bashy drop-in…" >&2
GOOS=linux GOARCH="$GOARCH" go build -o "$BIN" ./cmd/bash || { echo "modernish-suite: go build failed" >&2; exit 0; }
trap 'rm -f "$BIN"' EXIT

# bashy first (the shell-under-test), then the reference shells. Each entry is
# label=command. modernish is invoked as: <command> bin/modernish --test -qq
SHELLS="bashy=/bashy bash=bash dash=dash mksh=mksh zsh=zsh yash=yash"

echo "##### modernish self-test (regression/feature suite) vs bashy + reference shells #####"
echo "# INFO suite: modernish is strict about its host shell; not all shells initialise."
echo "# OK=succeeded  FAIL=failed-unexpectedly  (skip / xfail=known-shell-bug shown in parens)"

$OCI run --rm -e LANG=C -e SHELLS="$SHELLS" \
  -v "$MT:/msh:ro" -v "$BIN:/bashy:ro" "$IMG" bash -c '
  export LANG=C
  # Run from a writable copy so the loader can derive MSH_PREFIX cleanly.
  cp -r /msh /work 2>/dev/null && cd /work || cd /msh
  for spec in $SHELLS; do
    label=${spec%%=*}; cmd=${spec#*=}
    if ! command -v "$cmd" >/dev/null 2>&1; then
      printf "  %-8s (not found)\n" "$label"; continue
    fi
    out=$(timeout 420 "$cmd" bin/modernish --test -qq 2>&1)
    total=$(printf "%s\n" "$out" | sed -n "s/^Out of \([0-9][0-9]*\) test.*/\1/p" | head -1)
    if [ -z "$total" ]; then
      # modernish never reached the test run: init/parse failure. Report the
      # first diagnostic line so the blocker is visible, do NOT fake a number.
      blk=$(printf "%s\n" "$out" \
        | grep -iE "must be followed|syntax error|Fatal shell bug|cannot run modernish|Initialisation failed|does not run" \
        | head -1 | sed "s/^[[:space:]]*//; s/[[:space:]]*$//")
      [ -n "$blk" ] || blk="modernish init failed (no test summary emitted)"
      printf "  %-8s OK=0 FAIL=0 (skip=0 xfail=0) -> 0%% pass [BLOCKED: %s]\n" "$label" "$blk"
      continue
    fi
    ok=$(printf    "%s\n" "$out" | sed -n "s/^- \([0-9][0-9]*\) succeeded.*/\1/p"          | head -1)
    skip=$(printf  "%s\n" "$out" | sed -n "s/^- \([0-9][0-9]*\) .*skipped.*/\1/p"          | head -1)
    xfail=$(printf "%s\n" "$out" | sed -n "s/^- \([0-9][0-9]*\) failed expectedly.*/\1/p"  | head -1)
    fail=$(printf  "%s\n" "$out" | sed -n "s/^- \([0-9][0-9]*\) failed unexpectedly.*/\1/p"| head -1)
    : "${ok:=0}" "${skip:=0}" "${xfail:=0}" "${fail:=0}"
    pct=0; [ "$total" -gt 0 ] && pct=$((ok*100/total))
    printf "  %-8s OK=%s FAIL=%s (skip=%s xfail=%s of %s) -> %s%% pass\n" \
      "$label" "$ok" "$fail" "$skip" "$xfail" "$total" "$pct"
  done
' 2>&1 | grep -vE 'WARN|sysctl|level=warning'

echo "# Note: dash/yash failing modernish init is modernish's own verdict (unsupported host),"
echo "#       not a bashy result. bashy's line reflects bashy as a bash-5.3 drop-in."
exit 0
