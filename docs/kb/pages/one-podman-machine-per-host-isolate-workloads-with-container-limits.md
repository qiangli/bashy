---
id: 01a1b809-648a-742e-a644-afb68f50a43a
seq: 16
form: page
type: fact
title: One podman machine per host; isolate workloads with container limits
description: 'Do not plan on several concurrent podman machines on one host; isolate with per-container limits (outpost sandbox unit settings) — WHEN sizing podman for agents, sandboxes or k3s on a workstation (keywords: podman, podman machine, vm, concurrent, isolation, limits, sandbox, memory, cpus, pids)'
tags:
    - podman
    - sandbox
    - isolation
status: candidate
evidence: '2026-11-07: podman machine concurrency researched (fixed VM MAC, shared host sockets); sandbox settings per outpost/docs/settings.md and bashy service list; the example set command was not run'
source:
    tool: claude
created: "2026-11-07T22:03:05Z"
---

- Podman machine runs one VM at a time per host: machines share a fixed VM MAC and host-side sockets, so a second concurrent machine is not a supported setup.
- Keep ONE podman machine per host and isolate workloads with container limits instead. Outpost's sandbox unit exposes them as settings: `max_memory_mb`, `cpus`, `pids_limit`, `max_containers`, `allowed_images`:

```sh
outpost builtins set --service sandbox=on \
  --service-set sandbox.settings.max_memory_mb=2048 \
  --service-set sandbox.settings.cpus=2 \
  --service-set sandbox.settings.pids_limit=512
```

- For a plain container, the same idea: `podman run --memory 2g --cpus 2 --pids-limit 512 ...`.
- Size the one machine for the sum of what runs at once.
