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

Provide a candidate-only mode for scripts/posix-parity.sh that runs every probe directly through Bashy --posix under the selected local Bashy shell, never starts an OCI runtime, records each exit code and combined output, and clearly avoids claiming parity or certification. Use it for Sprint 274 Windows and macOS survey receipts. Preserve default differential behavior for later Linux/oracle work; do not change Bashy product semantics.
