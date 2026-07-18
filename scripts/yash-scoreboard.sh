#!/usr/bin/env bash
# yash POSIX (-p) scoreboard — the yash analogue of `make test-bash`.
# Runs every shell-agnostic *-p.tst against bashy AND real bash in one container
# (same env), per testcase, and reports:
#   - bashy pass rate
#   - the BASHY-SPECIFIC failures: cases where bash PASSES but bashy FAILS
#     (those are genuine bashy bugs to fix; cases where both fail are
#      posix-vs-default-bash noise, not our problem).
# Job-control / signal suites are excluded (goroutine-model ceiling). A few
# userland-dependent cases are also excluded from the shell-only denominator:
# ppid-p.tst:5 and simple-p.tst:290/299.
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
# shellcheck disable=SC2016
# Mounted at /sh, not /bashy: the binary enters strict POSIX mode only when its
# argv[0] base name is `sh` (invokedAsSh, internal/cli/main.go). Measuring the
# POSIX scoreboard through a testee named `bashy` would silently exercise the
# non-strict path. The result LABEL stays "bashy" so the tally schema is unchanged.
if ! $OCI run --rm -v "$ROOT/.yash-tests/tests:/yt:ro" -v "$ROOT/bin/.bashy-full:/sh:ro" \
  localhost/posix-shells-broad busybox ash -c '
  export LANG=C; cp -r /yt /work 2>/dev/null; cd /work
  excluded_case() {
    case "$1:$2" in
      ppid-p.tst:5|simple-p.tst:290|simple-p.tst:299) return 0;;
    esac
    return 1
  }
  for f in *-p.tst; do
    case "$f" in sig*|bg-p*|fg-p*|job-p*|kill*|wait-p*|testtty-p*|async-p*|intl*|unicode*) continue;; esac
    for pair in "bashy=/sh" "bash=bash"; do
      lbl=${pair%%=*}; sh=${pair#*=}
      command -v "$sh" >/dev/null 2>&1 || [ -x "$sh" ] || continue
      timeout -s KILL 8 busybox ash run-test.sh "$sh" "$f" >/dev/null 2>&1
      trs="${f%.tst}.trs"; [ -f "$trs" ] || continue
      # format: %%% OK[PASSED]: suite-p.tst:LINE: description
      # ERROR[PASSED_UNEXPECTEDLY] means the shell passed an upstream TODO
      # case; for bashy conformance triage, classify it as OK rather than a
      # behavioral failure.
      sed -nE \
        -e "s/^%%% OK\[[^]]*\]: ([^ ]*\.tst):([0-9]+): (.*)/\1|\2|$lbl|OK|\3/p" \
        -e "s/^%%% ERROR\[PASSED_UNEXPECTEDLY\]: ([^ ]*\.tst):([0-9]+): (.*)/\1|\2|$lbl|OK|\3/p" \
        -e "s/^%%% ERROR\[FAILED\]: ([^ ]*\.tst):([0-9]+): (.*)/\1|\2|$lbl|ERROR|\3/p" \
        "$trs" | while IFS="|" read -r file line label verdict desc; do
          excluded_case "$file" "$line" && continue
          printf "%s|%s|%s|%s|%s\n" "$file" "$line" "$label" "$verdict" "$desc"
        done
    done
  done
' > "$RAW"
then
  echo "yash-scoreboard: oracle runtime failed: $OCI run localhost/posix-shells-broad ..." >&2
  exit 2
fi
if [ ! -s "$RAW" ]; then
  echo "yash-scoreboard: no verdicts produced; check image localhost/posix-shells-broad and mounted yash tests" >&2
  exit 2
fi

# host-side: tally + bashy-specific failures (bash OK, bashy ERROR)
awk -F'|' '
  $3=="bashy"{by[$1"|"$2]=$4; desc[$1"|"$2]=$5}
  $3=="bash"{ba[$1"|"$2]=$4}
  END{
    okc=0; erc=0;
    for(k in by){ if(by[k]=="OK")okc++; else erc++ }
    print "=== yash -p scoreboard (bashy) ===" > "/dev/stderr";
    printf "bashy: %d pass / %d fail (of %d)  = %d%%\n", okc, erc, okc+erc, (okc+erc?okc*100/(okc+erc):0) > "/dev/stderr";
    n=0;
    for(k in by){ if(by[k]=="ERROR" && ba[k]=="OK"){ split(k,a,"|"); print a[1], a[2], desc[k]; n++ } }
    printf "BASHY-SPECIFIC FAILURES (bash OK, bashy FAIL): %d\n", n > "/dev/stderr";
  }
' "$RAW" | sort > "$OUT/failures.txt"

echo "--- failures by suite ---" >&2
awk '{c[$1]++} END{for(s in c) print c[s], s}' "$OUT/failures.txt" | sort -rn >&2
echo "full list: $OUT/failures.txt" >&2
