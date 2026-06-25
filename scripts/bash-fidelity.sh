#!/usr/bin/env bash
# bash-fidelity.sh — drop-in fidelity probe: where does our pure-Go `bash`
# diverge from GNU Bash 5.3 *specifically*?
#
# This is the high-signal complement to the 5-shell differential
# (scripts/oils-diff.sh). Because `bash` (cmd/bash) is a Bash DROP-IN — not
# merely a POSIX shell — the behavior that matters is matching BASH, including
# bash extensions. The 5-shell consensus marks every bash-only behavior
# "ambiguous" and hides exactly those gaps; here we compare ONLY our `bash`
# vs the real bash:5.3, so a difference is a real fidelity gap to close (or a
# deliberate, documented divergence).
#
# Two design choices keep the signal clean:
#   1. TWO-WAY (us vs bash:5.3 only). Shared-environment artifacts that make
#      every shell behave oddly (empty $TMP, busybox quirks) affect both sides
#      equally, so they cancel — unlike the consensus, which flags them.
#   2. PER-CASE ISOLATION. Each case runs in a FRESH cwd with HOME, TMP, and
#      $SH set to that dir, sequentially for both shells. This removes the
#      cross-case state pollution (a leftover $TMP/fifo, a prior cd's $OLDPWD)
#      that produced spurious "deviations" in the consensus harness.
#
#   localhost/posix-shells-oils = bash:5.3 + python3 + Oils helpers (argv.py).
#
# Usage: scripts/bash-fidelity.sh [oils-suite.test.sh ...]   (from repo root)
#   default: the broad POSIX/bash-core suite set below.
# Exit 0 iff zero DIFFs. Prints each DIFF (bash vs ours) + a fidelity count.
set -u
HERE=$(cd "$(dirname "$0")/.." && pwd)
OILS="$HERE/priorart/oils"
[ -d "$OILS/spec" ] || { echo "bash-fidelity: clone oils into priorart/oils first" >&2; exit 2; }

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif command -v ycode >/dev/null 2>&1; then OCI="ycode podman"
  fi
fi
[ -n "$OCI" ] || { echo "bash-fidelity: need a container runtime (docker / ycode podman)" >&2; exit 2; }

IMAGE=localhost/posix-shells-oils
if ! $OCI image exists "$IMAGE" 2>/dev/null; then
  echo "bash-fidelity: oils image missing — run scripts/oils-diff.sh once to build it" >&2
  exit 2
fi

# Broad POSIX + bash-core suites (skip ysh/osh-only, interactive, job-control).
if [ $# -gt 0 ]; then SUITES=("$@"); else
  SUITES=()
  for s in arith arith-context arith-dynamic array array-assign array-assoc \
           array-basic array-compat array-literal assign assign-extended \
           brace-expansion case_ command-sub dbracket dparen \
           builtin-printf builtin-vars builtin-getopts builtin-bracket \
           glob loop quote sh-func pipeline special-vars var-op-bash \
           var-op-strip var-op-test var-op-len var-op-patsub var-sub \
           word-split errexit redirect comments escape; do
    [ -f "$OILS/spec/$s.test.sh" ] && SUITES+=("$OILS/spec/$s.test.sh")
  done
fi
echo "bash-fidelity: ${#SUITES[@]} suites" >&2

CORPUS=$(mktemp -d)
python3 "$HERE/scripts/oils-proxy.py" --extract "$CORPUS" "${SUITES[@]}" >&2
CORPUS=$(cd "$CORPUS" && pwd)

ARCH=$($OCI run --rm "$IMAGE" uname -m | tr -d '\r')
case "$ARCH" in
  aarch64|arm64) GOARCH=arm64 ;;
  x86_64|amd64)  GOARCH=amd64 ;;
  *) echo "bash-fidelity: unsupported container arch '$ARCH'" >&2; exit 2 ;;
esac
BIN="$PWD/bin/.bash-linux-fidelity"
mkdir -p "$(dirname "$BIN")"
echo "bash-fidelity: building linux/$GOARCH bash…" >&2
GOOS=linux GOARCH="$GOARCH" go build -o "$BIN" ./cmd/bash || { echo "bash-fidelity: build failed" >&2; exit 2; }
trap 'rm -f "$BIN"; rm -rf "$CORPUS"' EXIT

$OCI run --rm -i -v "$BIN:/ours:ro" -v "$CORPUS:/corpus:ro" "$IMAGE" bash -s <<'INCONTAINER'
set -u
# runShell CMD SCRIPT — fresh, isolated env per run: a private cwd that is also
# $HOME/$TMP, and $SH pointing at the shell under test (many Oils cases use it).
# Sequential and independent, so neither side sees the other's filesystem state.
runShell() {
  local d out rc
  d=$(mktemp -d); cp "$2" "$d/s"
  out=$(cd "$d" && env -i HOME="$d" TMP="$d" PATH="$PATH" SH="$1" \
        timeout 10 "$1" ./s </dev/null 2>&1); rc=$?
  rm -rf "$d"
  # Each runShell call mints its OWN mktemp dir, so bash and ours get different
  # $d values; any test that echoes a $TMP/$HOME path would then differ purely
  # on the random dir name. Fold the per-run dir onto a constant so the
  # comparison measures behavior, not the harness's tmp path.
  out="${out//$d/TMPDIR}"
  printf '%s|%s' "$([ "$rc" -eq 0 ] && echo ok || echo err)" "$out"
}

match=0; diff=0
for SCRIPT in /corpus/*.sh; do
  base=$(basename "$SCRIPT")
  ref=$(runShell bash "$SCRIPT")
  ours=$(runShell /ours "$SCRIPT")
  # Normalize the $0/argv0 the harness invokes each shell with: bash runs as
  # "bash", ours is mounted+run as "/ours", so error prefixes read
  # "bash: line N:" vs "/ours: line N:". That is the binary NAME, not a
  # behavior difference — fold ours's name onto bash's so the comparison
  # measures shell behavior, not how this script invoked the binary.
  ours="${ours//\/ours/bash}"
  if [ "$ref" = "$ours" ]; then
    match=$((match+1))
  else
    diff=$((diff+1))
    echo "DIFF  $base"
    echo "   bash : [${ref}]"
    echo "   ours : [${ours}]"
  fi
done
total=$((match+diff)); pct="n/a"
[ "$total" -gt 0 ] && pct="$(( match*100/total ))%"
echo "=== $match match / $diff diff ($total cases) — bash-5.3 drop-in fidelity: $pct ==="
[ "$diff" -eq 0 ]
INCONTAINER
