---
id: 32e7d9379f9b
kind: task
title: Local lanes for the five disabled GitHub validation workflows
seq: 3198
status: todo
priority: p2
labels:
    - release
    - ci
created: 2026-11-09T00:53:56.157951Z
sprint: 599
sprint_id: b38d7a89-cadd-58e2-8748-66b0e9c2f201
sprint_title: 'S585 follow-ups: native Klatch on the Windows test host, rc.30 end-to-end and UAT, local gates under 30 min'
---

Operator rule (2026-11-08): GitHub runs only the binary release build. test, quickstart-scratch, conformance, airgap-image and install-matrix were disabled on 2026-11-08. Give every check they performed a local lane on the test hosts (reuse scripts/test-groups.sh and the self-container lane), fix the stale docs/airgap-image.md rows (sf, slack) found on the rc.28 tag, and keep the workflows disabled.
