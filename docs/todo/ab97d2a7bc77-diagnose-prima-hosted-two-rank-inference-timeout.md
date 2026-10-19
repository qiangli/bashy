---
id: ab97d2a7bc77
kind: bug
title: Diagnose Prima hosted two-rank inference timeout
seq: 1910
status: done
priority: p1
created: 2026-10-13T14:49:09.217959Z
weave: 88
assignee: codex-gpt6-astra
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T16:37:00.117881Z
closed_by: s436-codex-manager
---

Hosted candidate -4 run 38416093564: exact pin/native CMake/ring transport/single-rank inference pass on Linux amd64 and Darwin arm64, but two-rank rank1 times out at 90s on both. Read authenticated job logs and egress artifacts; both ranks reach warmup. Preserve two-rank fail-closed gate. In Bashy producer, add bounded diagnostic evidence (including syscall trace on Linux exercise failure and per-rank endpoint/status), determine if harness or first-party source defect, repair if demonstrable, update generated fork/tests/docs, commit with trailers. No dev-host native build/container/full suite; no push. If source defect, do not silently weaken or skip two-rank; report exact source patch needed and leave gate red.
