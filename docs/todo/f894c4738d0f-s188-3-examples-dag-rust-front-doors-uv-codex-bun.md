---
id: f894c4738d0f
kind: task
title: S188.3 examples/dag Rust front doors (uv, Codex, Bun) + smoke-dag-rust gate
seq: 288
status: todo
priority: p0
created: 2026-09-15T09:28:50.906856Z
sprint: 188
---

Three PR-ready dag.md graphs (uv, codex, bun) with the repo's own cargo/bun targets, a ~~~rs smoke fence (rs.uv()/rs.codex()/rs.bun() reading the workspace coordinates, cross-checked with shell builtins) and, for uv and Codex, a run target whose ~~~rs fence launches the cargo-built CLI; scripts/dag-rust-examples-smoke.sh + make smoke-dag-rust; README, docs/dag.md, CLAUDE.md. Gate: make smoke-dag-rust PASS on the installed binary against unchanged checkouts; smoke-dag-python + smoke-dag-typescript still PASS; CI=true make test.
