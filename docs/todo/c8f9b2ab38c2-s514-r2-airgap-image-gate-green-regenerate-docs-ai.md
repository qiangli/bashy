---
id: c8f9b2ab38c2
kind: task
title: 'S514.R2 airgap-image gate green: regenerate docs/airgap-image.md'
seq: 2729
status: todo
priority: p0
labels:
    - release
    - ci
created: 2026-10-27T07:53:02.615601Z
sprint: 514
sprint_id: fadd1a46-0368-50f1-a65d-4ed147b17629
sprint_title: 'invitation email + scoped satellites (KISS/MVP: scope + expiry on the one-time pairing code; no outpost change)'
---

airgap-image.yml has failed on every rc tag since rc.13. Measured rows are fine (rows=423 works=341 present=24 offline-by-design=57 fail=0); the FAIL is the doc table drift: docs/airgap-image.md differs from what the gate measured (new verb rows aws, az; act row text). Regenerate with AIRGAP_WRITE_DOC=1 on native Linux (amd64; confirm arm64 agrees — the az row reads 'present' on both lanes, check it is right, an absent CLI may belong in 'not usable offline'). Gate: airgap-image workflow green on both native lanes (workflow_dispatch or on the next tag).
