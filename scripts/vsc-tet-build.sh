#!/usr/bin/env bash
# vsc-tet-build.sh — SKELETON for the licensed VSC-PCTS2016 run (handoff Step 2–3).
#
# Builds the TET3 + VSXgen harness in a Linux container and wires bashy's `bash`
# drop-in as the System Under Test (SUT) in POSIX mode, scenario scoped to the
# shell + POSIX builtins. See docs/posix-cert-handoff-runbook.md (Steps 2–3) and
# docs/vsc-pcts-readiness.md (Scope).
#
# STATUS: skeleton. The tarball-specific build commands (marked `TODO(tarball)`)
# cannot be finalized until the licensed VSC-PCTS2016 bundle is in hand — its
# exact directory layout, configure flags, and VSXgen scenario file names come
# with it. This script encodes the KNOWN structure so finishing it is filling in
# blanks, not designing from scratch. Per the repo ethos, the TODOs FAIL LOUDLY
# rather than silently approximating.
#
# Usage:
#   VSC_TARBALL=/path/to/VSC-PCTS2016.tar.gz scripts/vsc-tet-build.sh
#
# Inputs (env):
#   VSC_TARBALL  path to the licensed suite tarball (gitignored; never commit it)
#   OCI          container runtime (default: docker, else `bashy podman`)
#   GOARCH       SUT build arch (default: container arch)
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)

# --- 0. Preconditions ---------------------------------------------------------
: "${VSC_TARBALL:?set VSC_TARBALL to the licensed VSC-PCTS2016 tarball path}"
[ -f "$VSC_TARBALL" ] || { echo "error: VSC_TARBALL not found: $VSC_TARBALL" >&2; exit 2; }

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif command -v bashy >/dev/null 2>&1; then OCI="bashy podman"
  else echo "error: need docker or bashy podman" >&2; exit 2; fi
fi

# Gate: never start the licensed run from a red baseline (runbook precondition).
echo ">>> pre-flight gate (must be green before spending the 12-month license)…" >&2
( cd "$ROOT" && make test-bash ) | tail -1
( cd "$ROOT" && scripts/posix-certdryrun.sh ) | grep -E "VERDICT" || true

ARCH=$($OCI run --rm debian:trixie uname -m | tr -d '\r')
case "$ARCH" in aarch64|arm64) GOARCH=${GOARCH:-arm64};; x86_64|amd64) GOARCH=${GOARCH:-amd64};; *) GOARCH=${GOARCH:-arm64};; esac

# $HOME is mounted into the podman/docker VM; /tmp generally is not.
WORK="$HOME/.cache/vsc-pcts"; mkdir -p "$WORK"
cp "$VSC_TARBALL" "$WORK/suite.tar.gz"

# --- 1. SUT: cross-build bashy's `bash` drop-in as /usr/bin/sh ----------------
echo ">>> building SUT: cmd/bash (linux/$GOARCH) as sh…" >&2
( cd "$ROOT" && GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=0 go build -o "$WORK/sh" ./cmd/bash )

# --- 2. Build TET3 + VSXgen + configure the scenario in a container -----------
# glibc base (Debian) — VSC-PCTS assumes a conventional POSIX/Linux build host.
cat > "$WORK/build-harness.sh" <<'INNER'
#!/bin/sh
set -eu
cd /work
mkdir -p suite && tar -xzf suite.tar.gz -C suite

# TODO(tarball): TET3 (Test Environment Toolkit) build. Typically:
#   cd suite/tet3   && ./configure ... && make install   (or the bundled buildtools)
# Confirm the actual path/recipe from the licensed bundle's INSTALL/README.
echo "TODO(tarball): build TET3 from the licensed bundle layout" >&2; exit 3

# TODO(tarball): VSXgen build + scenario generation (the VSC shell suite plugs in).
#   cd suite/vsxgen && ./setup.sh ... ; vsxgen ... to generate the run tree.

# TODO(tarball): SCOPE the scenario to the shell + POSIX builtins ONLY.
#   VSC-PCTS covers the shell language AND ~160 standalone utilities. Without
#   scoping, the run mostly tests the host's ls/grep/sed/… (out of bashy's
#   scope — see conformance-statement.md). Point the `sh` SUT at /work/sh and
#   either exclude the utility assertions or route them to the coreutils track.

# TODO(tarball): wire the SUT — install /work/sh as the shell under test
#   (e.g. ln -sf /work/sh /usr/bin/sh) and set TET_ROOT / the journal config so
#   the suite invokes bashy in POSIX mode (sh-invocation or --posix).
INNER
chmod +x "$WORK/build-harness.sh"

echo ">>> launching harness build in $OCI (debian:trixie)…" >&2
$OCI run --rm -v "$WORK:/work" debian:trixie sh -c '
  apt-get update -qq && apt-get install -y -qq build-essential >/dev/null 2>&1
  exec /work/build-harness.sh'

# --- 3. Next (manual, runbook Step 4–5) --------------------------------------
cat <<EOF >&2

Harness build skeleton ran. Remaining (runbook Steps 4–5):
  - Step 4: execute the shell + builtin subset under TET; capture journals.
  - Step 5: triage each non-PASS into real-bug (fix in ../sh, gated make
            test-bash 86/86) / declared-limitation (conformance-statement.md) /
            scope-excluded (standalone utility -> coreutils track).
Finish the TODO(tarball) blocks above once the licensed bundle layout is known.
EOF
