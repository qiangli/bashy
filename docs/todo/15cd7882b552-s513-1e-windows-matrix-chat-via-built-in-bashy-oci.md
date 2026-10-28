---
id: 15cd7882b552
kind: feature
title: S513.1e Windows Matrix chat via built-in Bashy OCI with persistent service and real E2E
seq: 2751
status: todo
priority: p0
labels:
    - windows
    - matrix-chat
    - oci
    - deferred
    - operator-stop
    - operator-stopped
created: 2026-10-28T02:51:04.947446Z
weave: 312
---

Operator 2026-10-28 requirement: Matrix chat must work on Windows, macOS, Linux. Signed tuwunel origin/main 5306cf07 builds only Linux amd64/arm64 and Darwin amd64/arm64; upstream supports RocksDB only. Implement Windows fallback through built-in bashy OCI/podman using the existing Bashy-signed Linux tuwunel asset; preserve native Linux/macOS path. Persist RocksDB and media outside container; support setup, account provisioning/shim, start/stop/restart/status, backup/restore, host reboot recovery; retain safe defaults and verified signed supply chain. Test on an explicitly claimed Windows peer using real signed service, two distinct users exchanging text/image, restart persistence and backup/restore; report any feature or environment blocker truthfully. No OCI execution on Dragon. Coordinate Windows host claims with S514/S515/S517; preserve shared checkouts and cross-sprint WIP. Delivery commit with Sprint/Story/Story-ID trailers; worker must sprint claim/submit, manager verifies and accepts.
