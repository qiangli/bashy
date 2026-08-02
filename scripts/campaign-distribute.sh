#!/usr/bin/env bash
# Fan a CAMPAIGN/free-suite test corpus across peer-joined workers, collect
# per-chunk results as evidence, and reduce them to one verdict.
#
# THE CERTIFICATION LINE (non-negotiable, see
# docs/distributed-campaign-verdict-equivalence.md): certification runs on
# ONE unchunked, exclusive system-under-test. This script's entire purpose is
# distribution, so MODE=cert must refuse outright, before any other argument
# is even looked at. Only the campaign arm and free suites may run here.
#
# Subcommands:
#   manifest   — (re)generate a deterministic chunk manifest for a corpus
#   distribute — fan the corpus across workers, evidence-check, reduce
#   serial     — run the whole corpus in one exclusive chunk (development
#                baseline for equivalence checking — NOT the certification
#                run; the certification run is the existing serial harness,
#                e.g. `make test-bash`)
#   verify     — run both serial and distribute, then assert the per-test
#                outcome SETS are identical (not just the totals)
#
# Required env for manifest/distribute/verify:
#   SUITE          campaign/free-suite label, e.g. "free-utils-sweep"
#   CASES_FILE     newline-delimited test-case IDs (the corpus; never suite
#                  content — a "free suite"'s redistribution terms bind its
#                  content, not this script; see docs/distributed-campaign-verdict-equivalence.md)
#   CHUNKS         number of chunks to split the corpus into
#   WORKERS        space-separated peer-worker role names, e.g. "worker-a worker-b"
#   RUN_CHUNK_CMD  command invoked as `$RUN_CHUNK_CMD <chunk_cases_file> <worker>`;
#                  must print exactly one `<test_id> <outcome>` line per case in
#                  the chunk file to stdout, and exit 0 iff it actually ran.
#                  outcome is one of: pass fail skip
#                  (used by `serial` always, and by `distribute` only under the
#                  test-injection seam below)
#
# Transport (distribute/verify):
#   CAMPAIGN_TRANSPORT=k8s   the ONLY real transport: one Kubernetes Job per
#                            chunk on the PEER cluster, pinned to distinct
#                            worker nodes — see scripts/campaign-distribute-k8s.sh
#                            for its env (WORKER_NODES, CHUNK_IMAGE,
#                            CHUNK_POD_CMD, NS, CHUNK_TIMEOUT).
#   CAMPAIGN_TEST_FAKE_TRANSPORT=1
#                            unit-injection seam ONLY: runs RUN_CHUNK_CMD in a
#                            local subprocess so the gate stays deterministic
#                            and network-free. Loud, and structurally
#                            non-promotable: the run emits
#                            CAMPAIGN_FAKE_TRANSPORT_VERDICT, never
#                            CAMPAIGN_VERDICT. Not a real execution mode.
#   With neither set, `distribute` refuses: there is no default transport.
#
# Optional:
#   SEED       manifest determinism seed (default: 0)
#   MANIFEST   path to read/write the chunk manifest (default: generated
#              into OUT_DIR — pass an explicit path to replay a disagreement)
#   OUT_DIR    scratch dir for chunk results + manifest (default: mktemp -d)
set -euo pipefail

# Unset (no colon) so a truly-absent MODE still defaults to the normal
# execution mode, but an explicitly empty MODE="" is preserved as empty and
# falls through to the allowlist check below, which refuses it.
MODE="${MODE-campaign}"

# Allowlist, not denylist: refuse to distribute unless MODE, after
# normalizing whitespace and case, is exactly "campaign". An unrecognized
# value — empty, misspelled, wrong case, whitespace-padded — is treated as
# cert-like and refused, never as campaign-like and allowed. This was an
# exact-string denylist on "cert" that MODE=CERT, MODE=certification, and
# MODE=" cert" all bypassed to distribute and exit 0.
mode_norm="$(printf '%s' "$MODE" | tr '[:upper:]' '[:lower:]' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
if [ "$mode_norm" != campaign ]; then
  echo "campaign-distribute: REFUSED — MODE must be exactly 'campaign' (whitespace/case-insensitive); got MODE='$MODE'. This script's entire purpose is distribution, and certification runs on one unchunked, exclusive SUT." >&2
  echo "campaign-distribute: see docs/distributed-campaign-verdict-equivalence.md; use the serial harness (e.g. make test-bash) for a certification result instead." >&2
  exit 77
fi

subcmd="${1:-distribute}"

campaign_root=$(cd "$(dirname "$0")" && pwd)

# --- transport selection ------------------------------------------------------
#
# The only real transport is k8s: per-chunk Kubernetes Jobs on the PEER
# cluster via scripts/campaign-distribute-k8s.sh (which consumes bashy#27/D3's
# dks-profile.sh through a single shim — cluster selection is never
# reimplemented here). The local-subprocess dispatch that W2 (#28) shipped
# survives ONLY as the CAMPAIGN_TEST_FAKE_TRANSPORT unit-injection seam: it is
# not reachable without that explicit test knob, and a run using it (or a
# fake kubectl injected via CAMPAIGN_K8S_FAKE_KUBECTL) can never emit the
# promotable CAMPAIGN_VERDICT marker.
campaign_transport() {
  if [ -n "${CAMPAIGN_TEST_FAKE_TRANSPORT:-}" ]; then
    printf 'injected-fake\n'
    return 0
  fi
  case "${CAMPAIGN_TRANSPORT:-}" in
    k8s) printf 'k8s\n' ;;
    '') printf 'none\n' ;;
    *) printf 'unknown\n' ;;
  esac
}

# Human/JSON-facing transport description, and the fake/real split that picks
# the verdict marker.
campaign_transport_desc() {
  case "$(campaign_transport)" in
    injected-fake) printf 'injected-fake\n' ;;
    k8s)
      if [ -n "${CAMPAIGN_K8S_FAKE_KUBECTL:-}" ]; then
        printf 'k8s-fake-kubectl\n'
      else
        printf 'k8s-peer\n'
      fi
      ;;
    *) printf 'none\n' ;;
  esac
}

campaign_transport_is_fake() {
  case "$(campaign_transport_desc)" in
    k8s-peer) return 1 ;;
    *) return 0 ;;
  esac
}

# Marker for the distribute verdict line: only a real peer-transport run may
# say CAMPAIGN_VERDICT — a fake-transport run is a logic check and must never
# be reportable as a distributed result.
campaign_verdict_marker() {
  if campaign_transport_is_fake; then
    printf 'CAMPAIGN_FAKE_TRANSPORT_VERDICT'
  else
    printf 'CAMPAIGN_VERDICT'
  fi
}

campaign_evidence_class() {
  if campaign_transport_is_fake; then
    printf 'logic-check-only'
  else
    # Even the real thing: distributed results are development evidence only,
    # never a certification result.
    printf 'development-only'
  fi
}

# --- peer dispatch shim -----------------------------------------------------
#
# One chunk -> one worker. k8s: create/await/collect/delete a per-chunk Job
# pinned to the worker's node (campaign-distribute-k8s.sh owns the kubectl
# side). injected-fake: the W2 local-subprocess path, test-injection only.
campaign_dispatch_chunk() {
  worker="$1" chunk_cases_file="$2" out_file="$3" chunk_id="$4" evidence_dir="$5"
  case "$(campaign_transport)" in
    k8s)
      "$campaign_root/campaign-distribute-k8s.sh" dispatch-chunk \
        "$worker" "$chunk_cases_file" "$out_file" "$chunk_id" "$evidence_dir"
      ;;
    injected-fake)
      "$RUN_CHUNK_CMD" "$chunk_cases_file" "$worker" >"$out_file"
      ;;
    *)
      echo "campaign-distribute: no transport selected — set CAMPAIGN_TRANSPORT=k8s (the only real transport). The local fake is a unit-test injection seam (CAMPAIGN_TEST_FAKE_TRANSPORT=1), never an execution mode." >&2
      exit 8
      ;;
  esac
}

# --- deterministic chunk manifest -------------------------------------------
#
# test_id -> chunk_id is derived from a stable CRC (cksum) of "$SEED:$test_id"
# mod CHUNKS, sorted by test_id — no $RANDOM, no reliance on fleet capacity or
# arrival order, so the exact same manifest is reproduced from the same
# (SEED, CASES_FILE, CHUNKS) on any host, and a disagreement can be replayed
# by pointing MANIFEST at the committed file that produced it.
campaign_build_manifest() {
  cases_file="$1" chunks="$2" seed="$3" manifest="$4"
  : >"$manifest"
  while IFS= read -r tc; do
    [ -n "$tc" ] || continue
    h=$(printf '%s' "${seed}:${tc}" | cksum | cut -d' ' -f1)
    chunk_id=$((h % chunks))
    printf '%s\t%s\n' "$chunk_id" "$tc" >>"$manifest"
  done <"$cases_file"
}

# Print the test-case IDs assigned to one chunk, sorted.
campaign_chunk_cases() {
  manifest="$1" chunk_id="$2"
  awk -F'\t' -v c="$chunk_id" '$1 == c { print $2 }' "$manifest" | sort
}

# --- corpus / manifest setup shared by distribute + serial + verify --------
campaign_require_common() {
  : "${SUITE:?set SUITE to the campaign/free-suite label}"
  : "${CASES_FILE:?set CASES_FILE to the newline-delimited case-id corpus}"
  [ -s "$CASES_FILE" ] || {
    echo "campaign-distribute: CASES_FILE is missing or empty — absence of evidence" >&2
    exit 2
  }
}

campaign_out_dir() {
  if [ -n "${OUT_DIR:-}" ]; then
    mkdir -p "$OUT_DIR"
    printf '%s\n' "$OUT_DIR"
  else
    mktemp -d
  fi
}

# --- distribute --------------------------------------------------------------
campaign_distribute() {
  campaign_require_common
  : "${CHUNKS:?set CHUNKS to the chunk count}"
  : "${WORKERS:?set WORKERS to space-separated peer-worker role names}"
  case "$CHUNKS" in
    ''|*[!0-9]*) echo "campaign-distribute: CHUNKS must be a positive integer" >&2; exit 2 ;;
  esac
  [ "$CHUNKS" -gt 0 ] || { echo "campaign-distribute: CHUNKS must be > 0" >&2; exit 2; }

  # shellcheck disable=SC2206
  workers_arr=($WORKERS)
  nworkers=${#workers_arr[@]}
  [ "$nworkers" -gt 0 ] || { echo "campaign-distribute: WORKERS is empty" >&2; exit 2; }

  transport="$(campaign_transport)"
  case "$transport" in
    k8s) ;;
    injected-fake)
      : "${RUN_CHUNK_CMD:?CAMPAIGN_TEST_FAKE_TRANSPORT needs RUN_CHUNK_CMD}"
      echo "campaign-distribute: ================================================================" >&2
      echo "campaign-distribute: FAKE TRANSPORT INJECTED (CAMPAIGN_TEST_FAKE_TRANSPORT) — this is" >&2
      echo "campaign-distribute: a reduction-logic check in a local subprocess. NOTHING here ran" >&2
      echo "campaign-distribute: on a peer worker; the output is NOT a distributed result." >&2
      echo "campaign-distribute: ================================================================" >&2
      ;;
    *)
      echo "campaign-distribute: no transport selected — set CAMPAIGN_TRANSPORT=k8s (the only real transport). The local fake is a unit-test injection seam (CAMPAIGN_TEST_FAKE_TRANSPORT=1), never an execution mode." >&2
      exit 8
      ;;
  esac

  out_dir="$(campaign_out_dir)"

  if [ "$transport" = k8s ]; then
    # Verify the role->node pinning is real distribution BEFORE creating any
    # Job, and guarantee cleanup of every Job the run creates even if it is
    # interrupted mid-flight (each dispatch also deletes its own Job; this
    # ledger sweep is the second line of defense, the Job TTL the third).
    "$campaign_root/campaign-distribute-k8s.sh" preflight
    trap '"$campaign_root/campaign-distribute-k8s.sh" cleanup "$out_dir/jobs.created" || true' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
  fi

  manifest="${MANIFEST:-$out_dir/manifest.tsv}"
  if [ -n "${MANIFEST:-}" ] && [ -s "$MANIFEST" ]; then
    echo "campaign-distribute: replaying committed manifest $MANIFEST" >&2
  else
    campaign_build_manifest "$CASES_FILE" "$CHUNKS" "${SEED:-0}" "$manifest"
  fi

  total_cases=$(sort -u "$CASES_FILE" | grep -c .)
  covered="$out_dir/covered.txt"
  : >"$covered"
  verdict="$out_dir/verdict.tsv"
  : >"$verdict"

  chunk_id=0
  while [ "$chunk_id" -lt "$CHUNKS" ]; do
    worker_idx=$((chunk_id % nworkers))
    worker="${workers_arr[$worker_idx]}"
    chunk_cases="$out_dir/chunk-$chunk_id.cases"
    campaign_chunk_cases "$manifest" "$chunk_id" >"$chunk_cases"
    expected_n=$(grep -c . "$chunk_cases" || true)

    result_file="$out_dir/chunk-$chunk_id.result"
    if [ "$expected_n" -eq 0 ]; then
      # Nothing assigned, nothing to dispatch — an empty-because-empty chunk
      # is positively accounted for as such, not inferred from silence.
      echo "campaign-distribute: chunk=$chunk_id has no assigned cases — skipping dispatch" >&2
      : >"$result_file"
      chunk_id=$((chunk_id + 1))
      continue
    fi

    if ! campaign_dispatch_chunk "$worker" "$chunk_cases" "$result_file" "$chunk_id" "$out_dir" 2>"$out_dir/chunk-$chunk_id.err"; then
      echo "campaign-distribute: FAIL chunk=$chunk_id worker=$worker — dispatch failed (worker unreachable or executor error)" >&2
      cat "$out_dir/chunk-$chunk_id.err" >&2 || true
      exit 3
    fi

    [ -s "$result_file" ] || {
      echo "campaign-distribute: FAIL chunk=$chunk_id worker=$worker — empty result file, expected $expected_n cases. Absence of evidence, not a pass." >&2
      exit 4
    }

    if [ "$transport" = k8s ]; then
      # Placement + identity evidence for this chunk must exist: the observed
      # node file and the evidence sidecar are written by dispatch only after
      # the pod's Job-ownership, node-pin, and evidence-header checks pass.
      if [ ! -s "$out_dir/chunk-$chunk_id.node" ] || [ ! -s "$out_dir/chunk-$chunk_id.evidence.json" ]; then
        echo "campaign-distribute: FAIL chunk=$chunk_id worker=$worker — no placement/identity evidence collected; an unattributed result is not evidence" >&2
        exit 4
      fi
    fi

    # The reported case set for this chunk must equal exactly the assigned
    # set: no dropped case, no foreign/duplicate case smuggled in from
    # elsewhere. A total-count match across chunks is not sufficient — see
    # the module doc's "loses 3, gains 3" example.
    got_cases="$(cut -f1 -d' ' "$result_file" 2>/dev/null | sort -u)"
    want_cases="$(sort -u "$chunk_cases")"
    if [ "$got_cases" != "$want_cases" ]; then
      echo "campaign-distribute: FAIL chunk=$chunk_id worker=$worker — reported case set does not match assigned set" >&2
      diff <(printf '%s\n' "$want_cases") <(printf '%s\n' "$got_cases") >&2 || true
      exit 5
    fi

    cut -f1 -d' ' "$result_file" >>"$covered"
    cat "$result_file" >>"$verdict"
    chunk_id=$((chunk_id + 1))
  done

  # Every case in the corpus must be positively accounted for exactly once
  # across all chunks — not merely that the totals reconcile.
  dupes="$(sort "$covered" | uniq -d)"
  if [ -n "$dupes" ]; then
    echo "campaign-distribute: FAIL — case(s) reported by more than one chunk:" >&2
    printf '%s\n' "$dupes" >&2
    exit 6
  fi
  missing="$(comm -23 <(sort -u "$CASES_FILE") <(sort -u "$covered"))"
  if [ -n "$missing" ]; then
    echo "campaign-distribute: FAIL — case(s) never reported by any chunk (missing evidence):" >&2
    printf '%s\n' "$missing" >&2
    exit 7
  fi

  if [ "$transport" = k8s ]; then
    # Distribution means distinct workers actually executed: the observed
    # (API-reported) placement across all dispatched chunks must cover at
    # least two distinct nodes. Two chunks on one worker is not distribution.
    observed_nodes=$(cat "$out_dir"/chunk-*.node 2>/dev/null | sort -u | grep -c . || true)
    if [ "$observed_nodes" -lt 2 ]; then
      echo "campaign-distribute: FAIL — dispatched chunks executed on $observed_nodes distinct node(s); distribution requires at least 2. Two chunks landing on one worker is not distribution." >&2
      exit 12
    fi
  fi

  covered_n=$(sort -u "$covered" | grep -c .)
  pass_n=$(awk '$2=="pass"' "$verdict" | grep -c . || true)
  fail_n=$(awk '$2=="fail"' "$verdict" | grep -c . || true)
  skip_n=$(awk '$2=="skip"' "$verdict" | grep -c . || true)
  class=pass
  [ "$fail_n" -eq 0 ] || class=fail

  sort -o "$verdict" "$verdict"
  printf '%s:{"schema":"bashy.campaign/v1","suite":"%s","mode":"distribute","transport":"%s","evidence":"%s","chunks":%s,"workers":%s,"cases":%s,"pass":%s,"fail":%s,"skip":%s,"class":"%s","manifest":"%s","verdict":"%s"}\n' \
    "$(campaign_verdict_marker)" "$SUITE" "$(campaign_transport_desc)" "$(campaign_evidence_class)" \
    "$CHUNKS" "$nworkers" "$covered_n" "$pass_n" "$fail_n" "$skip_n" "$class" "$manifest" "$verdict"
}

# --- serial (development baseline, NOT a certification run) ---------------
#
# Serial is local by definition — one exclusive chunk on this host, run
# directly through RUN_CHUNK_CMD, never through the distribution transport.
# It exists as the equivalence baseline for `verify`; the certification
# baseline remains the existing unchunked harness (e.g. make test-bash).
campaign_serial() {
  campaign_require_common
  : "${RUN_CHUNK_CMD:?set RUN_CHUNK_CMD to the per-chunk executor}"

  out_dir="$(campaign_out_dir)"
  all_cases="$out_dir/serial.cases"
  sort -u "$CASES_FILE" >"$all_cases"
  result_file="$out_dir/serial.result"

  if ! "$RUN_CHUNK_CMD" "$all_cases" "serial-exclusive" >"$result_file" 2>"$out_dir/serial.err"; then
    echo "campaign-distribute: FAIL serial run — executor failed" >&2
    cat "$out_dir/serial.err" >&2 || true
    exit 3
  fi
  [ -s "$result_file" ] || {
    echo "campaign-distribute: FAIL serial run — empty result file" >&2
    exit 4
  }

  got_cases="$(cut -f1 -d' ' "$result_file" | sort -u)"
  want_cases="$(cat "$all_cases")"
  if [ "$got_cases" != "$want_cases" ]; then
    echo "campaign-distribute: FAIL serial run — reported case set does not match corpus" >&2
    diff <(printf '%s\n' "$want_cases") <(printf '%s\n' "$got_cases") >&2 || true
    exit 5
  fi

  sort -o "$result_file" "$result_file"
  pass_n=$(awk '$2=="pass"' "$result_file" | grep -c . || true)
  fail_n=$(awk '$2=="fail"' "$result_file" | grep -c . || true)
  skip_n=$(awk '$2=="skip"' "$result_file" | grep -c . || true)
  class=pass
  [ "$fail_n" -eq 0 ] || class=fail
  printf 'CAMPAIGN_VERDICT:{"schema":"bashy.campaign/v1","suite":"%s","mode":"serial","transport":"local-serial","evidence":"development-only","chunks":1,"workers":1,"cases":%s,"pass":%s,"fail":%s,"skip":%s,"class":"%s","verdict":"%s"}\n' \
    "$SUITE" "$(grep -c . "$all_cases")" "$pass_n" "$fail_n" "$skip_n" "$class" "$result_file"
}

# --- verify: serial ≡ distributed, per-test outcome SET, not just totals ---
campaign_verify() {
  campaign_require_common
  : "${CHUNKS:?set CHUNKS to the chunk count}"
  : "${WORKERS:?set WORKERS to space-separated peer-worker role names}"
  : "${RUN_CHUNK_CMD:?set RUN_CHUNK_CMD to the per-chunk executor}"

  base_out="${OUT_DIR:-$(mktemp -d)}"
  mkdir -p "$base_out"
  serial_out="$base_out/serial"
  dist_out="$base_out/distribute"
  mkdir -p "$serial_out" "$dist_out"

  serial_line="$(OUT_DIR="$serial_out" campaign_serial)"
  dist_line="$(OUT_DIR="$dist_out" campaign_distribute)"

  serial_verdict=$(printf '%s\n' "$serial_line" | grep -o '"verdict":"[^"]*"' | cut -d'"' -f4)
  dist_verdict=$(printf '%s\n' "$dist_line" | grep -o '"verdict":"[^"]*"' | cut -d'"' -f4)

  if diff "$serial_verdict" "$dist_verdict" >"$base_out/diff.txt"; then
    echo "$serial_line" >&2
    echo "$dist_line" >&2
    # A fake-transport equivalence is a LOGIC claim about the reduction, not
    # evidence that distributed execution matched serial — mark it so.
    if campaign_transport_is_fake; then
      eq_marker=CAMPAIGN_LOGIC_EQUIVALENT
    else
      eq_marker=CAMPAIGN_VERDICT_EQUIVALENT
    fi
    printf '%s:{"schema":"bashy.campaign/v1","suite":"%s","transport":"%s","cases":%s}\n' \
      "$eq_marker" "$SUITE" "$(campaign_transport_desc)" "$(grep -c . "$serial_verdict")"
    return 0
  fi

  echo "campaign-distribute: MISMATCH — serial and distributed verdicts disagree on at least one test's outcome (equal totals do NOT imply equal verdicts):" >&2
  cat "$base_out/diff.txt" >&2
  printf 'CAMPAIGN_VERDICT_MISMATCH:{"schema":"bashy.campaign/v1","suite":"%s","diff":"%s"}\n' \
    "$SUITE" "$base_out/diff.txt"
  return 1
}

case "$subcmd" in
  manifest)
    campaign_require_common
    : "${CHUNKS:?set CHUNKS to the chunk count}"
    out_dir="$(campaign_out_dir)"
    manifest="${MANIFEST:-$out_dir/manifest.tsv}"
    campaign_build_manifest "$CASES_FILE" "$CHUNKS" "${SEED:-0}" "$manifest"
    printf '%s\n' "$manifest"
    ;;
  distribute) campaign_distribute ;;
  serial) campaign_serial ;;
  verify) campaign_verify ;;
  *)
    echo "campaign-distribute: unknown subcommand '$subcmd' (want: manifest|distribute|serial|verify)" >&2
    exit 2
    ;;
esac
