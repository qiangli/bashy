---
id: a8c4da9cf37d
kind: test
title: E1 Klatch end-to-end MVP across four satellite/outpost topologies under bashy oci
seq: 3148
status: todo
priority: p0
labels:
    - release
    - e2e
created: 2026-11-06T21:50:00.164534Z
assignee: muse-spark1.3
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
---

Klatch is already shipped; it is the test app for a full happy-path suite. Extend tests/klatch-e2e/run.py with --topology none|outpost|satellite|both.
- none: Klatch under bashy oci on the local host, local accounts only, fully hermetic (runs in the container gate).
- outpost: single-node outpost with no satellite (Sprint 565 O1 f7459c010953); BLOCKED if O1 is not met.
- satellite / both: self-hosted runs only; satellite endpoint and credentials from env/profile, never committed; outside public CI.
Journeys per topology: (a) human signup/login and agent auto-registration (own account, unique id, ordinary membership); (b) human and agent join one topic, post uniquely tagged messages and read each other's, attribution correct; (c) one bashy meet session with the human and agent. Klatch<->MB/Meet bridging is Sprint 568 D4 (design only): assert where it exists, else BLOCKED.
Keep the exit contract 0 pass / 1 fail / 2 BLOCKED (never a pass) and write one JSON evidence record per candidate SHA. Teardown removes sandbox and test accounts.
Fix in passing: DEFAULT_URL hard-codes a satellite address and a host name, which breaks the public-repo rule; make the URL required input and add the pattern to the scrub gate.
Deferred to Sprint 582: upgrade/rollback, credential-rotation and offline/reconnect legs; native macOS/Windows runs of the outpost topologies.
Verification: tests/klatch-e2e/test_run.py offline checks extended for topology selection and BLOCKED reporting; recorded runs of all four topologies on the frozen candidate.
