---
id: 77b364d18111
kind: task
title: S513.1f audit pinned tuwunel/RocksDB binary and OCI license obligations
seq: 2752
status: todo
priority: p0
labels:
    - license
    - matrix-chat
    - supply-chain
    - deferred
    - operator-stop
created: 2026-10-28T03:00:59.814907Z
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
---

Operator 2026-10-28 license compliance request. Audit exact signed tuwunel v1.9.4-bashy.2 source commit, Cargo.lock transitive RocksDB/Rust binding versions, selected RocksDB dual-license option and bundled third-party code/license metadata. Check what the distributed native binary and any proposed OCI image actually contain; reconcile LICENSE/NOTICE/source and attribution obligations with umbrella docs/bashy-service-supply-chain-policy.md and docs/persistence-license-policy.md. Apache option upstream is not sufficient evidence of artifact compliance. Record versions, direct source/lock/SBOM/release evidence and any missing notices. Repair only proven notice gaps via scoped commit/test; no paid builds unless needed. Windows OCI implementation remains STOPPED/deferred and no Windows support claim. Coordinate policy interpretation with s513-supervisor to avoid duplicate audit; preserve cross-sprint work. Delivery commit trailers Sprint/Story/Story-ID.
