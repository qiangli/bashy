---
id: ca801b586092
kind: release
title: 'S514.R3 bashy release via runbook #3 (kb:release-bashy)'
seq: 2730
status: todo
priority: p0
labels:
    - release
created: 2026-10-27T07:53:02.736207Z
sprint: 514
sprint_id: fadd1a46-0368-50f1-a65d-4ed147b17629
sprint_title: 'invitation email + scoped satellites (KISS/MVP: scope + expiry on the one-time pairing code; no outpost change)'
---

Manager-owned. After R1+R2 green: pick version (next free -dev + bare pair), release notes docs/releases/<v>.md, conformance test-bash 86/86 on the macOS test host, push vX.Y.Z-dev, verify 27-asset set, per-OS QA on published bytes (darwin owl, linux ephemeral droplet, windows otter), POSIX shell-only gate per runbook, then promote (operator confirm before the bare tag: promotion makes it the fleet target). Evidence: docs/sprint-514/release-<v>.md in the umbrella.
