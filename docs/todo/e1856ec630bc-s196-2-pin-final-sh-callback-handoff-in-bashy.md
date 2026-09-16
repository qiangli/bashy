---
id: e1856ec630bc
kind: task
title: S196.2 pin final sh callback handoff in Bashy
seq: 300
status: assigned
priority: p0
created: 2026-09-15T18:29:10.266134Z
assignee: codex-gpt5.6-sol
sprint: 196
---

Keep .sibling-pins sh at the final published Sprint 196 sh head, rerun warm zero-root example gates, and publish Bashy before umbrella integration.

The pinned mise CLI emits `<version>[-DEBUG] <platform> (<date>)`, without
its program name. Corrected the example's launcher assertion to compare the
entire first token with the manifest version or that exact version plus
`-DEBUG`; its reporting label remains `run: mise ...`. Wrong versions,
extra suffixes, prefixed names and empty output still fail. Checked the
actual built native CLI and release/debug positive and negative cases.
The retained integration attempt passed mise build and all 4072 unit tests
plus one additional test before failing the old launcher assertion; this
partial result is not a passing cold gate. Parent-owned full gates remain.
