---
id: 8656159eac97
kind: bug
title: Repair Windows Go by Example transpile deadline on arrays
seq: 327
status: todo
priority: p0
labels:
    - windows
    - go-by-example
    - transpile
created: 2026-09-23T12:43:19.161345Z
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

On named Windows host, Go by Example arrays one-row authenticated gate under unchanged 60s transpile deadline: oracle and interpreted pass, compiled transpile launches then reaches deadline with empty stdout/stderr and no output artifact. Reproduced twice with Bashy 5353ba3/sh 3d5559e and once with current public Bashy ca473fa/sh 111e307. Current candidate binary SHA-256 d3ab0c6590cea552da80c42d745811ad7cab5a555d031e9218194f7d3df03be0; manifest SHA-256 6d6526ca4b4a2e3cb652b4ee908482ab4393b0659b5cdb41c4e2a93bb5769cfb. Evidence on host under s250-88-head/evidence/gbe/results-current-smoke-1.jsonl.fail. Diagnose product transpile hang, preserve unchanged source/fixture/60s deadline, then rerun affected Windows gate after final source integration.
