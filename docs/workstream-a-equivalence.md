# Workstream A: chunked-suite equivalence proof

## Claim

A distributed run is equivalent to its serial reference when it executes the
same fixture set in the same suite and execution context, and produces the same
fixture-name-to-verdict mapping. Durations may differ and are not part of the
equivalence relation.

This is a verdict-equivalence claim, not a claim that logs, ordering, timing, or
host scheduling are identical.

## Evidence contract

Each chunk writes one schema-version-1 JSON record. All records admitted to one
aggregate must have:

- the same `suite`, `run_id`, and canonicalized `context`;
- the same positive `chunk.of` value;
- exactly one chunk index in the closed range `1..chunk.of`;
- no repeated fixture verdict name; and
- a verdict in `passed`, `failed`, `skipped`, or `timed_out` for every fixture.

Infrastructure status and preflight errors remain separate from conformance
verdicts. A missing or refused chunk is invalid evidence, not a skip.

`tools/bash53aggregate` rejects malformed, missing, duplicate, and
cross-context inputs before it creates a unified record. With `--reference`, it
then compares the aggregate and reference fixture sets and verdicts, ignoring
only `duration_seconds`.

## Method

1. Pin the code revision, suite revision, shell binary digest, container image
   digest, locale, exclusions, timeout policy, and manifest digest in `context`.
2. Choose a new `run_id` and use it for every chunk.
3. Run the serial reference with the same context and retain its JSON record.
4. Run every stable manifest chunk once. Do not replace a preflight failure
   with a synthetic skipped verdict.
5. Aggregate all chunk records with the pinned count:

   ```sh
   go run ./tools/bash53aggregate --expected 8 \
     --reference reference.json chunk-1.json chunk-2.json chunk-3.json \
     chunk-4.json chunk-5.json chunk-6.json chunk-7.json chunk-8.json
   ```

6. Treat exit zero as evidence of set and verdict equivalence. Archive the
   reference, chunk records, aggregate, manifests, and digests together.
7. Repeat once with chunk execution order randomized. This detects accidental
   shared-state or order dependence that a single matching run can miss.

For yash, first verify that `go run ./tools/yashsuite --list` and the committed
manifest cover the same fixture names exactly once. Stable chunks use
`--chunk=I/N`; `--of N --shard I` is an ad-hoc diagnostic partition and is not
evidence against a stable-manifest reference.

## Negative controls

The proof harness must also demonstrate rejection of one deliberately missing
chunk, a duplicate index, a changed `run_id`, a changed context field, and one
flipped reference verdict. Unit tests cover these controls.

## Current measurements

The Bash 5.3 corpus and `bin/bash` are not available in this workspace, so no
new suite run was made for this change. The existing measured `chunks.json`
records 86 passed, 0 failed, 0 skipped, and 0 timed out from its 2026-07-12
serial local run; that historical result is context only and is not presented
as a fresh equivalence proof.

The `.yash-tests/tests` runtime clone is also absent. `yash-chunks.json`
therefore uses an approximate equal-count eight-way partition with one-second
placeholder durations. Replace those estimates with real per-fixture timings
and rebalance deliberately after a complete run; do not silently change fixture
membership based on the current fleet size.
