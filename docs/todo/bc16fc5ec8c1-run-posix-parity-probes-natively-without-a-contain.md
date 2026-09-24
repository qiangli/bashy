---
id: bc16fc5ec8c1
kind: test
title: Run POSIX parity probes natively without a container oracle
seq: 342
status: todo
priority: p1
labels:
    - posix
created: 2026-09-24T19:27:22.936542Z
sprint: 274
sprint_id: e14729b9-4d75-5803-aca0-594e943ca2df
sprint_title: Bashy GNU Bash 5.3, POSIX mode and full Go tests across platforms
---

Provide native POSIX survey harnesses that run under Bashy GNU-compatible mode, never start OCI, and execute Bashy --posix as the subject. First run bashy check --prepare scoped to the parity and corpus scripts, then verify required harness utilities resolve inside Bashy; fail before any POSIX case if preload fails. Capture per-file corpus exits and outputs plus per-probe numeric exits and combined output without claiming parity or certification. Preserve default independent-oracle differential behavior and do not change Bashy product semantics.
