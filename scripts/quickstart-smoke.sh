#!/bin/sh
# Sprint 216, Story 540 — three-mode quickstart PROCESS-LEVEL check.
#
# Tests, as host processes (NOT containers):
#   (a) bashy + .bsh:               interpreted, no toolchain
#   (b) transpile --standalone:     Bash# → Go → binary, no bashy at runtime
#   (c) pre-prepared Python island: check --prepare then offline execution
#
# This is a fast host check. It is NOT the authoritative FROM-scratch proof —
# that is scripts/quickstart-container-smoke.sh, which builds and RUNS three
# real `FROM scratch` images, and the required Linux CI job that drives it. This
# script deliberately does not fake a container leg (an earlier version gzipped
# the transpiled Go SOURCE and built inside a golang image, proving neither a
# scratch image nor a run); it points at the real gate instead.
#
# Usage:
#   scripts/quickstart-smoke.sh [BASHY_BIN]      # uses PATH bashy when omitted
#   BASHY_BIN=bin/bashy scripts/quickstart-smoke.sh
#
# Claim: standalone binaries have a SMALLER SUPPLY-CHAIN SURFACE than
# interpreter-based invocations because they import only the plain shell
# runtime (mvdan.cc/sh/v3/lower/shellrt) and the Go standard library.
# They do NOT include a container engine or any external provider.
# External providers NOT present in the standalone binary:
#   podman, ollama, gh, loom, act, rclone, zot, seaweedfs, kopia, searxng
#
# Note: "smaller supply-chain surface" describes dependency reduction.
#       The binary is NOT sandboxed — it runs as the user without additional
#       isolation and is subject to the same OS permissions as any process.

set -eu

here=$(cd "$(dirname "$0")/.." && pwd -P)
bashy=${BASHY_BIN:-}
[ -n "$bashy" ] || bashy=$(command -v bashy 2>/dev/null || true)
[ -n "$bashy" ] && [ -x "$bashy" ] || {
    echo "quickstart-smoke: SKIP no bashy on PATH; set BASHY_BIN= to the installed binary" >&2
    exit 0
}

flag=${BASHSHARP_FLAG:---bashsharp}
rc=0

pass() { echo "quickstart-smoke: PASS $*"; }
fail() { echo "quickstart-smoke: FAIL $*" >&2; rc=1; }
skip() { echo "quickstart-smoke: SKIP $*"; }

# ── helpers ─────────────────────────────────────────────────────────────────

check_example() {
    ex=$1; expected_file=$2; extra_args=${3:-}
    # shellcheck disable=SC2086
    out=$("$bashy" $flag "$here/examples/quickstart/$ex.bsh" $extra_args 2>&1) || true
    want=$(cat "$here/examples/quickstart/$expected_file")
    if [ "$out" = "$want" ]; then
        pass "$ex (interpreted)"
    else
        fail "$ex (interpreted) got $(printf '%s' "$out" | head -3 | tr '\n' '|') want $(printf '%s' "$want" | head -3 | tr '\n' '|')"
    fi
}

# ── (a) bashy + .bsh ────────────────────────────────────────────────────────

check_example hello hello.expected
echo "quickstart-smoke: INFO  (a) bashy + .bsh — interpreted, no toolchain needed"

# ── (b) transpile --standalone ──────────────────────────────────────────────

if ! command -v go >/dev/null 2>&1; then
    skip "(b) transpile --standalone: go not on PATH; install Go to run this leg"
else
    tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/quickstart-standalone.XXXXXX")
    trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM

    if ! "$bashy" transpile "$flag" \
            "$here/examples/quickstart/hello_standalone.bsh" \
            --standalone -o "$tmpdir/main.go" 2>"$tmpdir/transpile.err"; then
        fail "(b) transpile: $(cat "$tmpdir/transpile.err")"
    else
        # Build the standalone binary (GOPROXY resolves the shell-runtime dep)
        if ! (cd "$tmpdir" && GOPROXY=direct GONOSUMDB='*' go mod tidy >"$tmpdir/tidy.log" 2>&1 \
              && go build -o hello . >"$tmpdir/build.log" 2>&1); then
            fail "(b) build failed — transpile.err: $(cat "$tmpdir/transpile.err") tidy: $(cat "$tmpdir/tidy.log") build: $(cat "$tmpdir/build.log")"
        else
            want=$(cat "$here/examples/quickstart/hello_standalone.expected")
            got=$(cd "$tmpdir" && ./hello 2>&1) || true
            if [ "$got" = "$want" ]; then
                pass "(b) transpile --standalone"
            else
                fail "(b) standalone binary output got='$got' want='$want'"
            fi

            # Record sizes (always, even without a container)
            uncompressed=$(wc -c < "$tmpdir/hello" | tr -d ' ')
            printf 'quickstart-smoke: SIZE  uncompressed=%s bytes\n' "$uncompressed"

            # Record one SBOM line from go version -m
            sbom_line=$(go version -m "$tmpdir/hello" 2>/dev/null \
                | grep 'mvdan.cc/sh' | head -1 | tr -s '\t' ' ') || true
            if [ -n "$sbom_line" ]; then
                printf 'quickstart-smoke: SBOM  %s\n' "$sbom_line"
            fi

            echo "quickstart-smoke: INFO  (b) standalone — smaller supply-chain surface:"
            echo "quickstart-smoke: INFO      no bashy, no container engine, no external provider at runtime"
        fi
    fi
fi

# ── (c) pre-prepared Python island ──────────────────────────────────────────

if ! command -v python3 >/dev/null 2>&1; then
    skip "(c) Python island: python3 not on PATH; run 'bashy check --prepare hello_island.bsh' first"
else
    # Provision (cache-first, downloads nothing on a repeat run)
    "$bashy" check --prepare "$here/examples/quickstart/hello_island.bsh" \
        >/dev/null 2>&1 || true
    check_example hello_island hello_island.expected
    echo "quickstart-smoke: INFO  (c) Python island — toolchain prepared via 'bashy check --prepare'"
fi

# ── The real FROM-scratch proof lives elsewhere ─────────────────────────────
#
# This script does NOT build container images. The authoritative proof — three
# real `FROM scratch` images built AND run — is:
#
#   scripts/quickstart-container-smoke.sh   (make smoke-quickstart-container)
#
# and the required Linux CI job .github/workflows/quickstart-scratch.yml, which
# is where the published compressed/uncompressed image sizes and the
# `go version -m` SBOM line come from. A pass here is a fast host check, not the
# release gate.
echo "quickstart-smoke: INFO  FROM-scratch images are proved by 'make smoke-quickstart-container' (or the Linux CI job), not here"

# ── Summary ──────────────────────────────────────────────────────────────────

if [ "$rc" -eq 0 ]; then
    echo "quickstart-smoke: OK"
else
    echo "quickstart-smoke: FAIL" >&2
fi
exit "$rc"
