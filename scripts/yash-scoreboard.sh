#!/usr/bin/env bash
# yash POSIX (-p) scoreboard — the yash analogue of `make test-bash`.
# Runs every shell-agnostic *-p.tst against bashy AND real bash in one container
# (same env), per testcase, and reports:
#   - bashy pass rate
#   - the BASHY-SPECIFIC failures: cases where bash PASSES but bashy FAILS
#     (those are genuine bashy bugs to fix; cases where both fail are
#      posix-vs-default-bash noise, not our problem).
# Job-control / signal suites are excluded (goroutine-model ceiling).
# Clean-room: clones yash at runtime, never vendors it (GPL).
set -u
cd "$(dirname "$0")/.."
ROOT=$PWD
OUT=${1:-/tmp/yash-scoreboard}
mkdir -p "$OUT"

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif [ -n "${BASHY:-}" ]; then OCI="$BASHY podman"
  elif command -v bashy >/dev/null 2>&1; then OCI="bashy podman"
  fi
fi
[ -n "$OCI" ] || { echo "need docker or bashy podman" >&2; exit 2; }

ARCH=$(uname -m); case "$ARCH" in aarch64|arm64) GOARCH=arm64;; *) GOARCH=amd64;; esac
echo "building linux/$GOARCH bashy…" >&2
GOOS=linux GOARCH=$GOARCH go build -o bin/.bashy-full ./cmd/bash || exit 2

RAW="$OUT/verdicts.raw"
echo "running yash -p corpus (bashy + bash, per case)…" >&2
$OCI podman ps -aq 2>/dev/null | xargs -r $OCI podman rm -f >/dev/null 2>&1 || true
# shellcheck disable=SC2016
$OCI run --rm -v "$ROOT/.yash-tests/tests:/yt:ro" -v "$ROOT/bin/.bashy-full:/bashy:ro" \
  localhost/posix-shells-broad busybox ash -c '
  export LANG=C; cp -r /yt /work 2>/dev/null; cd /work
  for f in *-p.tst; do
    case "$f" in sig*|bg-p*|fg-p*|job-p*|kill*|wait-p*|testtty-p*|async-p*|intl*|unicode*) continue;; esac
    for pair in "bashy=/bashy" "bash=bash"; do
      lbl=${pair%%=*}; sh=${pair#*=}
      command -v "$sh" >/dev/null 2>&1 || [ -x "$sh" ] || continue
      timeout -s KILL 8 busybox ash run-test.sh "$sh" "$f" >/dev/null 2>&1
      trs="${f%.tst}.trs"; [ -f "$trs" ] || continue
      # format: %%% OK[PASSED]: suite-p.tst:LINE: description
      sed -nE "s/^%%% (OK|ERROR)\[[A-Z]*\]: ([^ ]*\.tst):([0-9]+): (.*)/\2|\3|$lbl|\1|\4/p" "$trs"
    done
  done
' > "$RAW" 2>/dev/null

# host-side: tally + bashy-specific failures (bash OK, bashy ERROR)
awk -F'|' '
  $3=="bashy"{by[$1"|"$2]=$4; desc[$1"|"$2]=$5}
  $3=="bash"{ba[$1"|"$2]=$4}
  END{
    okc=0; erc=0;
    for(k in by){ if(by[k]=="OK")okc++; else erc++ }
    print "=== yash -p scoreboard (bashy) ===";
    printf "bashy: %d pass / %d fail (of %d)  = %d%%\n", okc, erc, okc+erc, (okc+erc?okc*100/(okc+erc):0) > "/dev/stderr";
    n=0;
    for(k in by){ if(by[k]=="ERROR" && ba[k]=="OK"){ split(k,a,"|"); print a[1], a[2], desc[k]; n++ } }
    printf "BASHY-SPECIFIC FAILURES (bash OK, bashy FAIL): %d\n", n > "/dev/stderr";
  }
' "$RAW" | sort > "$OUT/failures.txt"

echo "--- failures by suite ---" >&2
awk '{c[$1]++} END{for(s in c) print c[s], s}' "$OUT/failures.txt" | sort -rn >&2
echo "full list: $OUT/failures.txt" >&2
