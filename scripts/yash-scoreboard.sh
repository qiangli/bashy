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
#
# Denominator integrity (Sprint-7 test-p.tst incident): a suite preamble can set
# skip=true (e.g. test-p.tst probes `command -V test` for "built-in") and every
# case then logs "%%% SKIPPED: file:line: desc" — no [bracket] tag, unlike
# OK/ERROR — while run-test.sh still exits 0. Extracting only OK/ERROR made a
# fully-skipped or trs-less leg vanish from the tally, printing 100% over a
# silently shrunken denominator. So: SKIPPED marks are extracted too, a leg with
# rc!=0 or no trs emits a synthetic suite-level ERROR, and the summary checks
# bashy-measured vs oracle-measured case counts per suite — the gate FAILS on
# any delta. Acceptance basis: bashy case count == oracle case count, zero
# bashy-specific failures.
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
      trs="${f%.tst}.trs"
      rm -f "$trs"   # a stale trs from the other leg must not stand in for this one
      timeout -s KILL 8 busybox ash run-test.sh "$sh" "$f" >/dev/null 2>&1
      rc=$?
      if [ "$rc" -ne 0 ] || [ ! -f "$trs" ]; then
        # run-test.sh exits 0 even when cases fail; nonzero means a critical
        # error or the watchdog kill. A crashed or trs-less leg must not
        # silently vanish from the tally — emit a synthetic suite-level ERROR
        # (line 0) so the host-side denominator check sees the hole.
        if [ -f "$trs" ]; then trsstate=present; else trsstate=missing; fi
        printf "%s|0|%s|ERROR|suite-level: rc=%d trs=%s\n" "$f" "$lbl" "$rc" "$trsstate"
      fi
      [ -f "$trs" ] || continue
      # format: %%% OK[PASSED]: suite-p.tst:LINE: description
      #         %%% SKIPPED: suite-p.tst:LINE: description   (no [bracket] tag)
      # ERROR[PASSED_UNEXPECTEDLY] means the shell passed an upstream TODO
      # case; for bashy conformance triage, classify it as OK rather than a
      # behavioral failure. SKIPPED is extracted so a skip-preamble leg still
      # shows up in the denominator check instead of shrinking it silently.
      sed -nE \
        -e "s/^%%% OK\[[^]]*\]: ([^ ]*\.tst):([0-9]+): (.*)/\1|\2|$lbl|OK|\3/p" \
        -e "s/^%%% ERROR\[PASSED_UNEXPECTEDLY\]: ([^ ]*\.tst):([0-9]+): (.*)/\1|\2|$lbl|OK|\3/p" \
        -e "s/^%%% ERROR\[FAILED\]: ([^ ]*\.tst):([0-9]+): (.*)/\1|\2|$lbl|ERROR|\3/p" \
        -e "s/^%%% SKIPPED: ([^ ]*\.tst):([0-9]+): (.*)/\1|\2|$lbl|SKIPPED|\3/p" \
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

# host-side: tally + bashy-specific failures (bash OK, bashy ERROR) +
# denominator check (bashy-measured vs oracle-measured case counts per suite).
# SKIPPED verdicts and synthetic suite-level ERRORs (line 0) are counted apart
# from measured cases; any per-suite measured-count delta fails the gate.
awk -F'|' '
  { suites[$1]=1 }
  $4=="SKIPPED" { skipcnt[$3]++; next }
  $2=="0" { printf "SUITE-LEVEL ERROR: %s %s (%s)\n", $1, $3, $5 > "/dev/stderr" }
  $2!="0" { meas[$3]++; msuite[$3"|"$1]++ }
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
    printf "denominator: bashy measured %d cases (+%d skipped), oracle measured %d (+%d skipped)\n", \
      meas["bashy"]+0, skipcnt["bashy"]+0, meas["bash"]+0, skipcnt["bash"]+0 > "/dev/stderr";
    bad=0;
    for(s in suites){
      d = msuite["bash|"s]+0 - (msuite["bashy|"s]+0);
      if(d!=0){
        printf "DENOMINATOR DELTA %s: bashy=%d oracle=%d (delta %+d)\n", \
          s, msuite["bashy|"s]+0, msuite["bash|"s]+0, d > "/dev/stderr";
        bad=1;
      }
    }
    if(bad) print "yash-scoreboard: FAIL — bashy case count != oracle case count (cases went unmeasured)" > "/dev/stderr";
    exit bad;
  }
' "$RAW" > "$OUT/failures.unsorted"
GATE=$?
sort "$OUT/failures.unsorted" > "$OUT/failures.txt"
rm -f "$OUT/failures.unsorted"

echo "--- failures by suite ---" >&2
awk '{c[$1]++} END{for(s in c) print c[s], s}' "$OUT/failures.txt" | sort -rn >&2
echo "full list: $OUT/failures.txt" >&2
exit "$GATE"
