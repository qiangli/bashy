---
id: 2f1015f9b24d
kind: bug
title: outpost-profile add (outpost register --out) restarts the host's main outpost
seq: 3177
status: todo
priority: p1
labels:
    - outpost
created: 2026-11-07T21:52:03.045534Z
sprint: 565
sprint_id: 45140e29-11a7-5dac-b634-da49c692c9f6
sprint_title: 'Simultaneous satellites: one outpost instance per satellite'
---

Found 2026-11-07 (S587 C3) on native Windows and macOS: outpost-profile add NAME runs outpost register --out <profile>/agent.json, which prints 'Paired. Restarting the running outpost to apply' and restarts the host's MAIN outpost, although the new pairing goes to a profile file the main outpost does not read. The main satellite connection drops for a few seconds (on the dev box the main pid changed and reconnected; on the Windows host the published Klatch answered through both other satellites afterwards). Expected: a register with a non-default --out never restarts the running daemon (S497 already keeps it away from the host service). Smallest fix: outpost register skips the restart when --out is not the default config path (red/green test). Until then: run outpost-profile add outside live use, and note it in the kb runbook. KISS/MVP. Delivery commits carry Sprint, Story and Story-ID trailers.
