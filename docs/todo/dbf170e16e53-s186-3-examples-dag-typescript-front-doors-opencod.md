---
id: dbf170e16e53
kind: task
title: 'S186.3 examples/dag TypeScript front doors: opencode, openclaw, hermes-agent + smoke-dag-typescript gate'
seq: 286
status: done
priority: p1
created: 2026-09-15T08:10:31.562249Z
assignee: transom
sprint: 186
closed: 2026-09-15T08:17:19.275568Z
closed_by: transom
---

PR-ready dag.md for OpenCode (Bun workspace, Bun runtime), OpenClaw (pnpm workspace, Bun runtime for its NodeNext .js-for-.ts imports) and Hermes Agent (uv + npm workspace; one bashpp body with a ~~~py AND a ~~~ts fence, Node runtime); scripts/dag-typescript-examples-smoke.sh + make smoke-dag-typescript (OPENCODE_ROOT/OPENCLAW_ROOT/HERMESAGENT_ROOT required, BASHPP_BUN honoured, corepack pnpm shim); README + docs/dag.md; .sibling-pins sh bumped to the Sprint 184 runtime.
