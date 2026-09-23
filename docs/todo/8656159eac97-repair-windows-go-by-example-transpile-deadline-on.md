---
id: 8656159eac97
kind: bug
title: Repair Windows Go by Example transpile deadline on arrays
seq: 327
status: assigned
priority: p0
labels:
    - windows
    - go-by-example
    - transpile
created: 2026-09-23T12:43:19.161345Z
weave: 20
assignee: qiangli
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

On named Windows host, Go by Example arrays one-row authenticated gate under unchanged 60s transpile deadline: oracle and interpreted pass, compiled transpile launches then reaches deadline with empty stdout/stderr and no output artifact. Reproduced twice with Bashy 5353ba3/sh 3d5559e and once with current public Bashy ca473fa/sh 111e307. Current candidate binary SHA-256 d3ab0c6590cea552da80c42d745811ad7cab5a555d031e9218194f7d3df03be0; manifest SHA-256 6d6526ca4b4a2e3cb652b4ee908482ab4393b0659b5cdb41c4e2a93bb5769cfb. Evidence on host under s250-88-head/evidence/gbe/results-current-smoke-1.jsonl.fail. Diagnose product transpile hang, preserve unchanged source/fixture/60s deadline, then rerun affected Windows gate after final source integration.

Bounded diagnosis (weave #20/#21): a direct Windows transpile probe completed in 13.785s (first entry load 12.853s, second load 0.369s), while the authenticated one-row gate still hit its 60s transpile deadline. An ambient-environment overlay also reproduced a 60s stall before the first entry load completed; environment-group bisection provisionally implicated `SystemRoot`/`windir`. This is not yet the gate root cause: the gate supplies its own exact environment and launches through a Windows Job Object, so the overlay is not an equivalent launch. The exploratory double-load/product patches remain isolated and unmerged. One authenticated patched-candidate smoke still passed oracle (1.262s) and interpreted (9.634s) but timed out compiled transpile at 60.089s with no artifact; binary SHA-256 b4010b7fc4aca5ab92256b6ba817755e4c1bdb638ee9b9d3a21a93aee255a07e, manifest SHA-256 5b66f2393ebadaeffe9a68aa3b2666d9569e6aa85595cfe9377246d137429645. Keep this story open; prove the exact gate environment/launcher cause before changing product code. Full Windows Go by Example 255 and final-product rerun remain pending.
