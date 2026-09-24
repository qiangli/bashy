---
id: 8656159eac97
kind: bug
title: Repair Windows Go by Example transpile deadline on arrays
seq: 327
status: done
priority: p0
labels:
    - windows
    - go-by-example
    - transpile
created: 2026-09-23T12:43:19.161345Z
assignee: codex-s250
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
closed: 2026-09-23T20:29:39.692317Z
closed_by: codex-s250
---

On named Windows host, Go by Example arrays one-row authenticated gate under unchanged 60s transpile deadline: oracle and interpreted pass, compiled transpile launches then reaches deadline with empty stdout/stderr and no output artifact. Reproduced twice with Bashy 5353ba3/sh 3d5559e and once with current public Bashy ca473fa/sh 111e307. Current candidate binary SHA-256 d3ab0c6590cea552da80c42d745811ad7cab5a555d031e9218194f7d3df03be0; manifest SHA-256 6d6526ca4b4a2e3cb652b4ee908482ab4393b0659b5cdb41c4e2a93bb5769cfb. Evidence on host under s250-88-head/evidence/gbe/results-current-smoke-1.jsonl.fail. Diagnose product transpile hang, preserve unchanged source/fixture/60s deadline, then rerun affected Windows gate after final source integration.

Bounded diagnosis (weave #20/#21): a direct Windows transpile probe completed in 13.785s (first entry load 12.853s, second load 0.369s), while the authenticated one-row gate still hit its 60s transpile deadline. An ambient-environment overlay also reproduced a 60s stall before the first entry load completed; environment-group bisection provisionally implicated `SystemRoot`/`windir`. This is not yet the gate root cause: the gate supplies its own exact environment and launches through a Windows Job Object, so the overlay is not an equivalent launch. The exploratory double-load/product patches remain isolated and unmerged. One authenticated patched-candidate smoke still passed oracle (1.262s) and interpreted (9.634s) but timed out compiled transpile at 60.089s with no artifact; binary SHA-256 b4010b7fc4aca5ab92256b6ba817755e4c1bdb638ee9b9d3a21a93aee255a07e, manifest SHA-256 5b66f2393ebadaeffe9a68aa3b2666d9569e6aa85595cfe9377246d137429645. Keep this story open; prove the exact gate environment/launcher cause before changing product code. Full Windows Go by Example 255 and final-product rerun remain pending.

Root found in Sprint 250 follow-up: Go's Windows `os/exec` adds `SYSTEMROOT=C:\Windows` to the gate child even though the gate's recorded environment omits it. A paired diagnostic envdump showed that the fast direct .NET child lacks this key. With SYSTEMROOT present, a 15-second goroutine stack in the original 60-second arrays timeout names `sh/lower.goSDKCandidates → Bashy islandToolResolver → gotoolchain.Ensure → binmgr.extractTreeZip → syscall.CreateFile`: the explicit valid `BASHPP_GO` SDK is already available, but the importer still invokes the managed resolver and unpacks a second SDK. sh Story #148 (`754cc69f9385`) now returns the valid explicit SDK before resolver discovery at public sh `79f48367`; an isolated patched Windows binary transpiled the unchanged source SHA-256 `9b23202e95c943d209c6a60c61f2581c3b77137164f776fcfab2d5fe28f411cc` with SYSTEMROOT in 12.374 seconds under the unchanged 60-second deadline, producing Go and map. Raw receipts: `C:\Users\noviadmin\s250-327\diag\goroutines-systemroot.txt` and `stack-systemroot-patched\gate\result.json`. Authenticated final-product one-row/full Windows GBE gate remains pending; keep #327 open until it passes.

Final acceptance, 2026-09-23: the authenticated final Windows arrays row passed oracle, interpreted and compiled; compiled transpile completed in 1.245s under the original 60s limit with a valid source map. The full unchanged Go by Example corpus then passed **255/255** with independent validator PASS on Bashy `e48bd7df29a496692f22ebb72520f7f32c932697`, sh `0736c52ec82d39922359548f0496e44ec583c6ba`, and gate-time bashsharp-tests `eda9a22856288ebc239cb177c2d206cf69d51da4`. Semantic root `6867eacc716b234cd30f830d9ae01b93c7e9ea3c1b8b55d3a06aaa9ce36e154e`; ledger SHA-256 `7614c6adb597b2cb2a5024bc88ce5e00f158e8a18ede198584ab7574c872148f`. The source, fixture and deadline are unchanged.
