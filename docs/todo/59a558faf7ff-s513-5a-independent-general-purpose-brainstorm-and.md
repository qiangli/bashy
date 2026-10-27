---
id: 59a558faf7ff
kind: feature
title: S513.5a independent general-purpose brainstorm and browser E2E
seq: 2737
status: assigned
priority: p0
created: 2026-10-27T17:15:28.363553Z
weave: 297
assignee: muse-conductor
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
---

Implement code portion of parent 2dd13e52dda6. Brainstorm remains its own independent command/skill, server, default port, URL and data from whiteboard and Matrix; starts/works with either absent and alongside both. Generalize existing examples/brainstorm for configurable named sessions (title/prompts/columns/vote budget), generic facilitator and participant copy, private writing, reveal, voting, close and export; preserve old classroom record compatibility and owner/privacy checks. Add automated real-browser happy-path E2E: create session, at least three contexts/private ideas before reveal, reveal, votes constrained by budget, close, export and restart persistence, standalone and prefix. Keep scoped to examples/brainstorm and focused tests; no core command, no containers, no paid infra. Use native peer for browser work; do not restart other services or edit whiteboard/Matrix. Read Bashy CLAUDE.md. One bounded run, report exact gate commands/results and remaining integrated criteria. Commit as injected identity with Sprint513/Story/Story-ID trailers; manager gates and merges.
