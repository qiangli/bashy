---
id: 22c5c48f3234
kind: test
title: Gate optional-only updates on Bashy digest and certified route invariance
seq: 395
status: todo
priority: p0
labels:
    - architecture
    - certification
created: 2026-10-02T21:37:00.008052Z
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
---

Add release validation that separately versioned builtin/optional catalog updates leave the accepted base/core Bashy executable digest and Profile D route manifest byte-for-byte unchanged. Populate an extension ring and prove `VSC_PROFILE=cert` shell/command lookup, atlas claims, and applet dispatch exclude it. If digest, route, fence grammar or runner ABI changes, require a new base/core candidate and conformance-impact review; do not launch a full Profile D rerun before the 64-blocker and 9-INSPECT accounting gate. Verify rollback and offline cache behavior.
