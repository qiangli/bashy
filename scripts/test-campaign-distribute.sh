#!/usr/bin/env bash
# Deterministic tests for scripts/campaign-distribute.sh. No cluster, no
# network: every "peer worker" is a plain role name dispatched to a local
# fake executor script — this is the whole worker layer, faked.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
DISTRIBUTE="$root/scripts/campaign-distribute.sh"
tmp=$(mktemp -d)
trap '/bin/rm -rf "$tmp"' EXIT
adversarial_fail=0

fail() { printf '%s\n' "$1" >&2; exit 1; }
adversarial() { printf '%s\n' "$1" >&2; adversarial_fail=1; }

# --- corpus + canonical outcome map -----------------------------------------
cases="$tmp/cases.txt"
printf '%s\n' case-a case-b case-c case-d case-e case-f >"$cases"

outcomes="$tmp/outcomes.tsv"
cat >"$outcomes" <<'EOF'
case-a pass
case-b fail
case-c pass
case-d pass
case-e skip
case-f pass
EOF

# --- fake per-chunk executor: $1=chunk_cases_file $2=worker -----------------
# Reads the canonical outcome for each case in the chunk from $OUTCOME_MAP,
# with fault-injection knobs (all off by default) used by the adversarial
# tests below. Never touches the network or a real cluster.
fake_run_chunk="$tmp/fake-run-chunk.sh"
cat >"$fake_run_chunk" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
chunk_file="$1"
worker="$2"

if [ -n "${FAKE_UNREACHABLE_WORKER:-}" ] && [ "$worker" = "$FAKE_UNREACHABLE_WORKER" ]; then
  echo "fake-run-chunk: simulated unreachable worker $worker" >&2
  exit 9
fi

if [ -n "${FAKE_EMPTY_WORKER:-}" ] && [ "$worker" = "$FAKE_EMPTY_WORKER" ]; then
  exit 0
fi

while IFS= read -r id; do
  [ -n "$id" ] || continue
  if [ "$worker" != "serial-exclusive" ] && [ "$id" = "${FAKE_DROP_CASE:-}" ]; then
    continue
  fi
  outcome=$(awk -v i="$id" '$1==i{print $2}' "$OUTCOME_MAP")
  if [ "$worker" != "serial-exclusive" ] && [ -n "${FAKE_SWAP_A:-}" ] && [ -n "${FAKE_SWAP_B:-}" ]; then
    if [ "$id" = "$FAKE_SWAP_A" ]; then
      outcome=$(awk -v i="$FAKE_SWAP_B" '$1==i{print $2}' "$OUTCOME_MAP")
    elif [ "$id" = "$FAKE_SWAP_B" ]; then
      outcome=$(awk -v i="$FAKE_SWAP_A" '$1==i{print $2}' "$OUTCOME_MAP")
    fi
  fi
  printf '%s %s\n' "$id" "$outcome"
done <"$chunk_file"

if [ "$worker" != "serial-exclusive" ] && [ -n "${FAKE_INJECT_FOREIGN_CASE:-}" ]; then
  printf '%s %s\n' "$FAKE_INJECT_FOREIGN_CASE" pass
fi
FAKE
chmod +x "$fake_run_chunk"

export OUTCOME_MAP="$outcomes"
export SUITE=free-utils-sweep
export CASES_FILE="$cases"
export RUN_CHUNK_CMD="$fake_run_chunk"

# =============================================================================
# 1. Certification line: MODE=cert refuses to distribute, unconditionally,
#    before anything else runs — even with a fully valid distribute config.
# =============================================================================
set +e
out=$(cd "$root" && MODE=cert CHUNKS=3 WORKERS="worker-a worker-b" \
  "$DISTRIBUTE" distribute 2>&1)
rc=$?
set -e
[ "$rc" -eq 77 ] || fail "cert mode did not refuse with exit 77 (got $rc): $out"
case "$out" in *REFUSED*) ;; *) fail "cert mode refusal did not say REFUSED: $out" ;; esac

set +e
out=$(cd "$root" && MODE=cert "$DISTRIBUTE" serial 2>&1)
rc=$?
set -e
[ "$rc" -eq 77 ] || fail "cert mode did not refuse 'serial' subcommand with exit 77 (got $rc)"

# =============================================================================
# 2. Manifest determinism: same (seed, corpus, chunks) -> byte-identical
#    manifest, on repeated generation.
# =============================================================================
m1="$tmp/m1.tsv"; m2="$tmp/m2.tsv"
(cd "$root" && CHUNKS=3 SEED=7 MANIFEST="$m1" "$DISTRIBUTE" manifest >/dev/null)
(cd "$root" && CHUNKS=3 SEED=7 MANIFEST="$m2" "$DISTRIBUTE" manifest >/dev/null)
diff "$m1" "$m2" >/dev/null || fail "manifest generation is not deterministic for the same seed"
[ "$(sort -u "$m1" | wc -l | tr -d ' ')" -eq 6 ] || fail "manifest does not cover all 6 cases exactly once"

# =============================================================================
# 3. Happy path distribute: verdict matches the canonical outcome map exactly.
# =============================================================================
out_dir="$tmp/happy"
result=$(cd "$root" && CHUNKS=3 SEED=1 WORKERS="worker-a worker-b worker-c" \
  OUT_DIR="$out_dir" "$DISTRIBUTE" distribute)
case "$result" in
  *'CAMPAIGN_VERDICT:'*'"cases":6'*'"pass":4'*'"fail":1'*'"skip":1'*'"class":"fail"'*) ;;
  *) fail "unexpected happy-path verdict: $result" ;;
esac

# =============================================================================
# 4. Manifest replay: pointing MANIFEST at a committed file reproduces the
#    exact same chunk assignment (a disagreement can be replayed).
# =============================================================================
replay_manifest="$tmp/replay.tsv"
(cd "$root" && CHUNKS=3 SEED=1 MANIFEST="$replay_manifest" "$DISTRIBUTE" manifest >/dev/null)
r1=$(cd "$root" && CHUNKS=3 WORKERS="worker-a worker-b worker-c" \
  MANIFEST="$replay_manifest" OUT_DIR="$tmp/replay1" "$DISTRIBUTE" distribute)
r2=$(cd "$root" && CHUNKS=3 WORKERS="worker-a worker-b worker-c" \
  MANIFEST="$replay_manifest" OUT_DIR="$tmp/replay2" "$DISTRIBUTE" distribute)
# Strip the OUT_DIR-dependent scratch-file paths before comparing — only the
# substantive per-test verdict must be reproducible, not the scratch layout.
strip_paths() { sed -e 's/"manifest":"[^"]*"/"manifest":X/' -e 's/"verdict":"[^"]*"/"verdict":X/'; }
diff "$tmp/replay1/verdict.tsv" "$tmp/replay2/verdict.tsv" >/dev/null \
  || fail "replaying the same committed manifest produced different per-test outcomes"
[ "$(printf '%s' "$r1" | strip_paths)" = "$(printf '%s' "$r2" | strip_paths)" ] \
  || fail "replaying the same committed manifest produced different verdict summaries"

# =============================================================================
# 5. Evidence invariant — unreachable worker fails the reduction, loudly.
# =============================================================================
if (cd "$root" && FAKE_UNREACHABLE_WORKER=worker-b CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/unreachable" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/unreachable.err"); then
  adversarial "an unreachable worker was silently accepted as a passing chunk"
fi
grep -q 'worker unreachable\|dispatch failed' "$tmp/unreachable.err" \
  || adversarial "unreachable-worker failure did not name the cause"

# =============================================================================
# 6. Evidence invariant — a worker that runs but returns nothing for a
#    non-empty chunk is absence of evidence, not a pass.
# =============================================================================
if (cd "$root" && FAKE_EMPTY_WORKER=worker-a CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/empty" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/empty.err"); then
  adversarial "an empty chunk result was silently accepted as a passing chunk"
fi

# =============================================================================
# 7. Evidence invariant — a chunk that drops one of its assigned cases must
#    fail the reduction (a missing case is not an implicit pass or skip).
# =============================================================================
if (cd "$root" && FAKE_DROP_CASE=case-c CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/drop" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/drop.err"); then
  adversarial "a chunk silently dropping an assigned case was accepted"
fi

# =============================================================================
# 8. Evidence invariant — a chunk injecting a case it was not assigned must
#    also fail (foreign evidence is not a substitute for the real one, and
#    letting it through would let a chunk over-report and mask a drop
#    elsewhere via a coincidentally matching total).
# =============================================================================
if (cd "$root" && FAKE_INJECT_FOREIGN_CASE=case-zz CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/foreign" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/foreign.err"); then
  adversarial "a chunk reporting a case outside its assigned set was accepted"
fi

# =============================================================================
# 9. Evidence invariant — a corrupted manifest that assigns one case to two
#    chunks must fail the reduction, even though each chunk faithfully
#    reports exactly its own (overlapping) assigned set.
# =============================================================================
dup_manifest="$tmp/dup-manifest.tsv"
cat >"$dup_manifest" <<'EOF'
0	case-a
0	case-b
1	case-c
1	case-d
2	case-e
2	case-f
2	case-a
EOF
if (cd "$root" && CHUNKS=3 WORKERS="worker-a worker-b worker-c" \
  MANIFEST="$dup_manifest" OUT_DIR="$tmp/dup" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/dup.err"); then
  adversarial "a manifest assigning one case to two chunks was accepted"
fi
grep -q 'more than one chunk' "$tmp/dup.err" || adversarial "duplicate-case failure did not name the cause"

# =============================================================================
# 10. Evidence invariant — a corrupted manifest that omits a case entirely
#     must fail closed: no chunk can report evidence for a case it was never
#     told to run.
# =============================================================================
missing_manifest="$tmp/missing-manifest.tsv"
cat >"$missing_manifest" <<'EOF'
0	case-a
0	case-b
1	case-c
1	case-d
2	case-e
EOF
if (cd "$root" && CHUNKS=3 WORKERS="worker-a worker-b worker-c" \
  MANIFEST="$missing_manifest" OUT_DIR="$tmp/missing" "$DISTRIBUTE" distribute \
  >/dev/null 2>"$tmp/missing.err"); then
  adversarial "a manifest omitting a corpus case was accepted"
fi
grep -q 'missing evidence\|never reported' "$tmp/missing.err" \
  || adversarial "missing-case failure did not name the cause"

# =============================================================================
# 11. verify: serial and distributed agree exactly -> equivalence holds.
# =============================================================================
out=$(cd "$root" && CHUNKS=3 SEED=1 WORKERS="worker-a worker-b worker-c" \
  OUT_DIR="$tmp/verify-ok" "$DISTRIBUTE" verify)
case "$out" in *'CAMPAIGN_VERDICT_EQUIVALENT:'*) ;; *) fail "verify did not report equivalence on matching runs: $out" ;; esac

# =============================================================================
# 12. verify — THE CORE DELIVERABLE: equal totals must not mask a set
#     mismatch. The fake executor swaps case-a and case-b's outcomes only in
#     the distributed path (a plausible bug: a worker mis-attributing which
#     case a result belongs to). Serial reports a=pass,b=fail; distributed
#     reports a=fail,b=pass. Both have 4 pass / 1 fail / 1 skip overall — a
#     count-only check would call this equivalent. It is not.
# =============================================================================
set +e
out=$(cd "$root" && FAKE_SWAP_A=case-a FAKE_SWAP_B=case-b CHUNKS=3 SEED=1 \
  WORKERS="worker-a worker-b worker-c" OUT_DIR="$tmp/verify-swap" "$DISTRIBUTE" verify 2>"$tmp/verify-swap.err")
rc=$?
set -e
if [ "$rc" -eq 0 ]; then
  adversarial "verify accepted a distributed run with the same totals but a different per-test outcome set"
fi
case "$out" in *'CAMPAIGN_VERDICT_MISMATCH:'*) ;; *) adversarial "verify mismatch did not report CAMPAIGN_VERDICT_MISMATCH: $out" ;; esac
grep -q 'case-a\|case-b' "$tmp/verify-swap.err" \
  || adversarial "verify mismatch diff did not name the disagreeing case(s)"

# =============================================================================
# 13. Zero-case chunks (CHUNKS > distinct cases) are handled without treating
#     an empty-because-nothing-assigned chunk as an evidence failure.
# =============================================================================
out=$(cd "$root" && CHUNKS=8 SEED=3 \
  WORKERS="worker-a worker-b worker-c worker-d" \
  OUT_DIR="$tmp/overchunked" "$DISTRIBUTE" distribute)
case "$out" in *'"cases":6'*) ;; *) fail "over-chunked corpus lost or gained cases: $out" ;; esac

[ "$adversarial_fail" -eq 0 ] || exit 1
echo "campaign-distribute verdict equivalence: PASS"
