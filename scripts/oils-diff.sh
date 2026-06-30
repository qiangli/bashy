#!/usr/bin/env bash
# oils-diff.sh — Gate-D refinement: run Oils spec-test CASE CODE through the live
# 5-shell same-environment differential (posix-diff.sh), NOT against Oils'
# baked-in expected output (which drifts by bash version + parser quirks). A
# DEVIATION here = bashy differs where the real shells agree = a true finding.
#
# Pipeline:
#   1. extract case code from the given Oils suites -> a temp corpus (one .sh/case)
#   2. build localhost/posix-shells-oils = posix-shells + python3 + Oils helpers
#      (argv.py/sh_init.py on PATH, py3 shebang) — many cases use argv.py
#   3. run posix-diff.sh over that corpus with the oils image
#
# Usage: scripts/oils-diff.sh [oils-suite.test.sh ...]   (run from repo root)
#   default: a curated set of POSIX/bash-core suites.
set -u
HERE=$(cd "$(dirname "$0")/.." && pwd)
OILS="$HERE/priorart/oils"
[ -d "$OILS/spec" ] || { echo "oils-diff: clone oils into priorart/oils first" >&2; exit 2; }

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif [ -n "${BASHY:-}" ]; then OCI="$BASHY podman"
  elif command -v bashy >/dev/null 2>&1; then OCI="bashy podman"
  fi
fi
[ -n "$OCI" ] || { echo "oils-diff: need a container runtime" >&2; exit 2; }

# Self-contained image: all 5 oracle shells + python3 + Oils helpers on PATH.
if ! $OCI image exists localhost/posix-shells-oils 2>/dev/null; then
  echo "oils-diff: building localhost/posix-shells-oils (5 shells + python3 + argv.py)…" >&2
  bd=$(mktemp -d)
  mkdir -p "$bd/bin"
  for h in argv.py sh_init.py; do
    [ -f "$OILS/spec/bin/$h" ] || continue
    sed '1s|.*|#!/usr/bin/env python3|' "$OILS/spec/bin/$h" > "$bd/bin/$h"; chmod +x "$bd/bin/$h"
  done
  cat > "$bd/Containerfile" <<EOF
FROM bash:5.3
RUN apk add --no-cache dash yash zsh mksh python3
COPY bin/ /usr/local/bin/
EOF
  $OCI build -q -t localhost/posix-shells-oils "$bd" >&2 || { echo "build failed" >&2; exit 2; }
  rm -rf "$bd"
fi

# Default curated POSIX/bash-core suites (skip osh/ysh-only + interactive).
if [ $# -gt 0 ]; then SUITES=("$@"); else
  SUITES=()
  for s in word-split quote var-op-test var-op-strip var-op-len var-op-bash arith \
           case_ loop assign command-sub builtin-cd builtin-echo builtin-printf \
           builtin-getopts redirect glob brace-expansion sh-func pipeline \
           special-vars exit-status; do
    [ -f "$OILS/spec/$s.test.sh" ] && SUITES+=("$OILS/spec/$s.test.sh")
  done
fi
echo "oils-diff: ${#SUITES[@]} suites" >&2

CORPUS=$(mktemp -d)
python3 "$HERE/scripts/oils-proxy.py" --extract "$CORPUS" "${SUITES[@]}" >&2

# Reuse the differential harness with the oils-helper image.
POSIX_SHELLS_IMAGE=localhost/posix-shells-oils bash "$HERE/scripts/posix-diff.sh" "$CORPUS"
status=$?
rm -rf "$CORPUS"
exit $status
