#!/usr/bin/env bash
# Run the local POSIX survey from Bashy's GNU-compatible shell.
# Usage: BASHY=/absolute/path/to/bashy bashy scripts/posix-native-survey.sh OUTDIR
# This is an execution survey; only an independent oracle can score parity.

BASHY=${BASHY:-./bin/bashy}
ROOT=${BASHY_POSIX_REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)} || exit 2
cd "$ROOT" || exit 2
[ -x "$BASHY" ] || { echo "posix-native-survey: Bashy is not executable: $BASHY" >&2; exit 2; }
[ "$#" -eq 1 ] || { echo "usage: $0 OUTDIR" >&2; exit 2; }
OUTDIR=$1
mkdir -p "$OUTDIR" || exit 2

echo "posix-native-survey: preloading before POSIX tests"
# $0 retains the host-native path on Windows. Go's check reader needs that
# spelling; Bashy's shell cwd may use /c/... while Go's os.Open does not.
"$BASHY" check --prepare "$0" > "$OUTDIR/preload.log" 2>&1 || {
  cat "$OUTDIR/preload.log" >&2
  echo "posix-native-survey: preload failed; no POSIX tests were started" >&2
  exit 2
}
"$BASHY" -c 'for tool in env sed tr grep head awk sleep dirname pwd rm cat mkdir; do
  command -v "$tool" >/dev/null || { printf "missing harness utility: %s\n" "$tool" >&2; exit 1; }
done' >> "$OUTDIR/preload.log" 2>&1 || {
  cat "$OUTDIR/preload.log" >&2
  echo "posix-native-survey: Bashy userland preload incomplete; no POSIX tests were started" >&2
  exit 2
}
echo "preload=pass; harness utilities resolved in Bashy" >> "$OUTDIR/preload.log"

pass=0
fail=0
: > "$OUTDIR/corpus.tsv"
for file in test/posix-corpus/*.sh; do
  name=${file##*/}
  "$BASHY" --posix "$file" > "$OUTDIR/${name%.sh}.out" 2>&1
  rc=$?
  printf '%s\t%s\n' "$name" "$rc" >> "$OUTDIR/corpus.tsv"
  if [ "$rc" -eq 0 ]; then pass=$((pass+1)); else fail=$((fail+1)); fi
done
printf 'corpus: %d files exited 0, %d nonzero\n' "$pass" "$fail"

export BASHY
"$BASHY" scripts/posix-parity.sh --candidate-only > "$OUTDIR/probes.log" 2>&1
probe_rc=$?
[ "$probe_rc" -eq 0 ] || { echo "posix-native-survey: probe harness failed (exit $probe_rc)" >&2; exit 2; }
tail -1 "$OUTDIR/probes.log"
echo "posix-native-survey: outputs: $OUTDIR"
[ "$fail" -eq 0 ]
