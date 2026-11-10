---
id: 46dbbf801155
kind: feature
title: 'Parity gap: no verb lists hosts attached to a satellite'
seq: 3209
status: wontfix
priority: p2
labels:
    - parity
created: 2026-11-10T14:02:11.028256Z
closed: 2026-11-10T14:20:38.412045Z
---

docs/ui-cli-parity.md satellite console row. bashy paired-host ls --satellite is refused (no GET /api/v1/satellites/assignment mapping on the master; proven by TestComposedSatelliteOperationsWorkflow). The daemon only lists its own attachments via outpost satellite list. Need a master-side verb (or the assignment mapping) so an agent on any machine can answer which hosts are attached to satellite X. --json.

Closed 2026-11-10 (manager): not a gap. `bashy paired-host ls --satellite ID` filters through the Bearer GET /api/v1/satellites/assignment (S606 S5, yoke 4445e9fe), which bashy pins.
