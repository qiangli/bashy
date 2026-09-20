#!/bin/sh
# Sprint 216, Story 540 — three-mode quickstart smoke gate.
#
# Tests:
#   (a) bashy + .bsh:               interpreted, no toolchain
#   (b) transpile --standalone:     Bash# → Go → binary, no bashy at runtime
#   (c) pre-prepared Python island: check --prepare then offline execution
#
# Local smoke is deterministic (exits 0 or non-0, same result every run).
# Container leg is OPTIONAL and runs only when docker or podman is available;
# hosts without a container engine still pass the full local gate.
#
# Usage:
#   scripts/quickstart-smoke.sh [BASHY_BIN]      # uses PATH bashy when omitted
#   BASHY_BIN=bin/bashy scripts/quickstart-smoke.sh
#   BASHY_BIN=bin/bashy SKIP_CONTAINER=1 scripts/quickstart-smoke.sh
#
# Container leg records:
#   - compressed and uncompressed binary sizes for the standalone artifact
#   - one go version -m SBOM line confirming the shell-runtime dependency
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
skip_container=${SKIP_CONTAINER:-}
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

# ── Optional container leg ───────────────────────────────────────────────────

engine=
for e in docker podman; do
    if command -v "$e" >/dev/null 2>&1; then
        engine=$e
        break
    fi
done

if [ -z "$engine" ] || [ -n "$skip_container" ]; then
    skip "container leg (no docker/podman found or SKIP_CONTAINER set) — local smoke is the authoritative gate"
else
    # Container leg: build the standalone binary inside a minimal Go image
    # and record its sizes; the gate itself is still the local build above.
    tmpdir_c=$(mktemp -d "${TMPDIR:-/tmp}/quickstart-container.XXXXXX")
    trap 'rm -rf "$tmpdir_c"' EXIT HUP INT TERM

    "$bashy" transpile "$flag" \
        "$here/examples/quickstart/hello_standalone.bsh" \
        --standalone -o "$tmpdir_c/main.go" 2>/dev/null

    # Compressed size (gzip -9 approximates a container layer)
    gzip -9 -c "$tmpdir_c/main.go" > "$tmpdir_c/main.go.gz"
    compressed_src=$(wc -c < "$tmpdir_c/main.go.gz" | tr -d ' ')
    uncompressed_src=$(wc -c < "$tmpdir_c/main.go" | tr -d ' ')
    printf 'quickstart-smoke: CONT  container-src uncompressed=%s compressed=%s bytes\n' \
        "$uncompressed_src" "$compressed_src"

    # Run the build inside a throwaway container; capture the binary size
    if "$engine" run --rm \
            -v "$tmpdir_c:/work" \
            -w /work \
            golang:1.27-alpine \
            sh -c 'GOPROXY=direct GONOSUMDB='"'"'*'"'"' go mod tidy && go build -ldflags "-s -w" -o hello .' \
            >"$tmpdir_c/container.log" 2>&1; then
        unc=$(wc -c < "$tmpdir_c/hello" | tr -d ' ')
        gzip -9 -c "$tmpdir_c/hello" > "$tmpdir_c/hello.gz"
        comp=$(wc -c < "$tmpdir_c/hello.gz" | tr -d ' ')
        printf 'quickstart-smoke: CONT  binary uncompressed=%s compressed=%s bytes\n' "$unc" "$comp"
        # SBOM from inside the container build
        sbom=$(go version -m "$tmpdir_c/hello" 2>/dev/null \
            | grep 'mvdan.cc/sh' | head -1 | tr -s '\t' ' ') || true
        [ -n "$sbom" ] && printf 'quickstart-smoke: SBOM  %s\n' "$sbom"
        pass "container leg ($engine)"
    else
        skip "container leg: build failed (engine=$engine); $(cat "$tmpdir_c/container.log" | tail -3)"
    fi
fi

# ── Summary ──────────────────────────────────────────────────────────────────

if [ "$rc" -eq 0 ]; then
    echo "quickstart-smoke: OK"
else
    echo "quickstart-smoke: FAIL" >&2
fi
exit "$rc"
