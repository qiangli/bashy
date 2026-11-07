---
id: 4c7687a19487
kind: enhancement
title: Bashy manages one podman machine per host; isolate workloads by container limits
seq: 3165
status: todo
priority: p2
labels:
    - sandbox
    - podman
    - windows
created: 2026-11-07T10:59:37.364822Z
---

Operator decision 2026-11-07 (from Sprint 565 coordination with Sprint 586): for cross-platform compatibility Bashy supports exactly one VM/podman machine per host. macOS runs only one podman machine at a time; on Windows every WSL2 machine and distro shares one VM, so a second machine gives no CPU/RAM isolation; Linux has no VM. One machine already serves rootless and rootful connections (<name> and <name>-root).

Problem today: the container engine (yoke/external/podman/engine) probes podman-machine-default first and otherwise provisions its own machine named bashy, so a host can end up with two machines. The sandbox and podman units each take one socket setting. A heavy gate has no supported place except the single machine, which on the Windows test host is 2 GiB and also runs the UAT cohort's Klatch container.

Change:
1. Adopt, don't add: use the host's existing podman machine and create one only when none exists; one naming rule, not a two-name probe.
2. Size the machine for the host (CPUs and memory at creation; resizing a running machine needs a restart, so do it only with operator approval; see F5).
3. Isolate workloads with container limits: every sandbox or gate container gets CPU, memory and pids caps (the sandbox unit already has max_memory_mb, cpus and pids_limit); heavy gates run as capped containers next to long-running apps.
4. Long-running app containers must come back at machine start (F25); forwarding published ports to the host matters for the single machine (F24).

Acceptance: a host with an existing machine gains no second one; a new host gets one sized machine; a gate container with caps cannot starve a long-running app container on the same machine (measured on Linux, macOS and native Windows). No change to running machines without an operator window.
