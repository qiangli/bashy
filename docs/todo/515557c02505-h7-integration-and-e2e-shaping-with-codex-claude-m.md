---
id: 515557c02505
kind: test
title: H7 Integration and E2E shaping with Codex, Claude, Muse and agy over CLI, MCP and REST
seq: 3080
status: todo
priority: p2
labels:
    - shaping
    - integration
    - e2e
created: 2026-11-05T02:57:51.661013Z
sprint: 564
sprint_id: 0f09b646-9acd-5bec-849d-2cf3b0e4edef
sprint_title: 'Software shaping for Bashy apps: any user shapes an app on their own host'
---

## Goal

Prove that supported third-party agentic tools can drive the same Software Shaping workflow through Bashy CLI, MCP and REST API, using one minimal Hello World web app. Tools: Codex, Claude, Muse and agy. This is a planning/backlog story only; filing it authorizes no implementation or test execution in the current session.

## Dependencies and scope

Depends on H1 (workflow and identity contract), H2 (CLI/shared-engine parity) and H4 (submission). Coordinate platform evidence with H5 rather than duplicating its full cross-platform suites. Exercise existing supported integrations and public interfaces. Confirm the registered tool names, versions, authentication requirements and actual shaping MCP surface before execution; any missing adapter or operation is an explicit blocking dependency, not a silently skipped or passing cell. Do not invent an MCP tool name or bypass shaping by editing the deployed app directly.

## Coverage matrix

Twelve tool-by-interface combinations: Codex, Claude, Muse and agy, each through (1) Bashy CLI, (2) the supported MCP shaping surface and (3) REST /api/shape. Each tool must actually invoke the selected interface. A direct HTTP harness alone does not establish a third-party-agent E2E pass. Record the supported host/platform for each cell and link reused H5 platform evidence by exact candidate identity.

## Shared fixture and happy path

Use a tiny app initially displaying exactly "Hello, World!", with a deterministic verifier and no external data dependency. For each cell, start from the same pinned source in a fresh isolated workspace and use the same request: change the displayed greeting to "Hello, Shaping!".

Exercise status/discovery, candidate creation (wish or the supported --from equivalent), show/diff, verify, preview, explicit approve, active-version status, undo/restore to the original greeting, and submission of an approved version through the normal review flow. Assert the rendered page in a real browser before and after activation and rollback; check the visible greeting and active version, not only command exit codes or agent narration. Submission must produce a reviewable tracked item/run with source identity and normal gates; no automatic merge is required.

## Integration and negative cases

- Assert consistent state transitions, records and verifier results across CLI/MCP/REST against the shared engine. Compare meaningful fields, allowing generated IDs and timestamps to differ.
- Verify automatic todo/story linkage, shaping origin, actor and represented-user identity, source commit, workspace, artifact/version identity and evidence links.
- A broken candidate that fails the verifier must never activate; the prior working app remains available.
- Denied authentication/authorization and wrong-scope access must fail without changing another user's active version or protected app parts.
- Agent timeout/interruption and retried requests must leave coherent, inspectable candidate state under the documented API contract; never silently approve, activate or merge.
- Test runs must not mutate shared checkouts. Keep secrets out of prompts, public logs and committed evidence; use existing isolated runners and clean up only each run's disposable resources.

## Acceptance and evidence

All twelve requested cells have real integration/E2E evidence and pass the common lifecycle assertions. Unsupported or unavailable cells remain explicitly blocked; no mock-only pass, unexplained skip or unverified completion. Preserve a compact matrix with tool/model version, interface, host role/OS, pinned app and Bashy candidate, exact commands or sanitized requests, raw logs, browser assertions and verifier evidence. Reuse existing runners and gates where possible. The manager reviews evidence before closing this story; a reproducible test recipe and fixture accompany the final delivery. This story complements H5 and does not close or replace it.

## User requirement: shaping must itself be shapeable

The bashy shape CLI and the Software Shaping app/UI must themselves support Software Shaping. A user or supported agent can shape the shaping tool through the same governed workflow; this is not limited to shaping the Hello World target app. Coordinate the self-shaping path with H3 and H5: preserve the released CLI/app and protected parts, verify candidates before activation, keep a working recovery path, and demonstrate status/identity plus one-step rollback. Include an integration/E2E self-shaping case using a small visible change to the shaping CLI/app, without bypassing verifier, authorization, isolation or normal submission gates. H3 is also a dependency for this self-shaping case. This records a requirement for future delivery only; no implementation or execution is authorized in this session.
