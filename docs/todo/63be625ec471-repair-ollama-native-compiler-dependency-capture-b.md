---
id: 63be625ec471
kind: bug
title: Repair Ollama native compiler dependency capture before hosted run
seq: 1925
status: done
priority: p1
created: 2026-10-13T17:23:31.158981Z
weave: 100
assignee: codex-gpt6-luna
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T17:29:20.433616Z
closed_by: s436-codex-manager
---

Preserve and build on worker commit 2c92a47d from weave #99; it is partial and not merged. For each actual cgo C/C++ compile, inject unique -MMD/-MF dependency file and retain it safely under parallel compilation; wrappers must preserve compiler semantics and capture generated Metal. Distinguish reviewed platform/toolchain headers from non-system module-cache or external native inputs; fail closed instead of silently dropping them. Add adversarial fake compiler/concurrent and outside-source dependency tests, run builds/check.sh ollama, commit worker-owned change with trailers, no push/no dev-host native build. Do not claim license clearance; existing story 5c4432 remains open for hosted five-target inventory and exact hash/license review.
