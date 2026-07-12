---
id: 150c70159a52
kind: task
title: 'P1: pin the conformance corpus manifest (chunks.json) so chunked runs are reproducible'
status: triaged
stage: test
priority: p0
reporter: qiangli
created: 2026-07-12T22:58:50.548754Z
weave: 11
---

GOAL OF THE CAMPAIGN: run the whole conformance matrix in MINUTES, not hours. Today
`make test-bash-parallel` is 86 fixtures / 127s in 5 groups on ONE host. The matrix
(bash-5.3 + yash + uutils + zsh + the differentials) is the part that takes hours.

This issue is P1 only. Do not attempt P2-P4.

## The one rule that makes or breaks this

CHUNK COUNT IS A CORPUS PROPERTY, PINNED IN A COMMITTED MANIFEST — NEVER DERIVED FROM
FLEET CAPACITY.

If chunk membership is computed from "how many hosts do I have right now", then
case→chunk assignment SHIFTS between runs, and two things break at once:
  - selective re-run ("just re-run chunk 3") stops meaning anything;
  - the fingerprint cache can never hit, because the unit it keys on keeps changing.
A 3-host run and a 7-host run must execute the SAME chunks; the fleet decides only how
many chunks run CONCURRENTLY.

## Deliverable

1. `chunks.json` at the repo root — a committed manifest mapping every bash-5.3
   fixture to a stable chunk id. Include a schema version.
2. `tools/bash53suite` reads it. THERE IS EXACTLY ONE FIXTURE RUNNER — do not add a
   second. (A second runner in shell is what silently hung CI for 20 minutes a run and
   let the gate go unmeasured for ~10 merges. Never again.)
3. The `dag.md` chunk lanes (`test-bash-chunks`, `test-bash-chunks-fleet`,
   `test-bash-chunks-container`) consume the manifest instead of splitting by worker
   count.
4. Balance the chunks by MEASURED duration, not by fixture count — makespan is set by
   the slowest chunk, and the fixtures are wildly uneven. Record the durations you
   measured in the manifest so a later pass can rebalance without re-measuring.
5. A test asserting the manifest COVERS EVERY FIXTURE EXACTLY ONCE. A fixture silently
   dropped from the manifest is a fixture that stops being tested, and the PASS count
   would not move — which is precisely the failure mode this project has already been
   bitten by.

## Non-negotiable

THE AUTHORITATIVE RUN STAYS SINGLE-HOST AND UNCHUNKED. `make test-bash` (86/86 serial)
remains the release gate. Chunked/campaign mode is for SPEED during development and
never speaks for the gate. Do not weaken, skip, or reinterpret any fixture to make a
chunk balance.

CI REFUSES ANY SKIPPED FIXTURE. A skip is silent coverage loss.

## Verify (this is the gate)

    make test-bash-parallel                      # still 86 passed, 0 failed, 0 skipped
    bashy dag test-bash-chunks                   # chunked lane agrees with serial
    go test ./tools/... ./internal/...
