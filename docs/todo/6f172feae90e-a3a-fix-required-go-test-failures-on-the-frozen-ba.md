---
id: 6f172feae90e
kind: bug
title: A3a Fix required go test ./... failures on the frozen baseline
seq: 3052
status: done
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T04:31:35.436483Z
weave: 11
assignee: claude-sonnet5.5
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
closed: 2026-11-04T04:55:09.679178Z
closed_by: claude-opus5.5
---

Child of A3 (3d4d3d262bdd), filed for independent ownership; A3 acceptance unchanged. Source: docs/s561/a2-results.md (A2 run #9, branch agent/weave-issue-9). Baseline = frozen tree (bashy main == 5a4bd03f outside docs), standalone GOWORK=off. Rules: smallest causal repair at the owning layer; red test first, then green; no skip, weakened assertion, timeout increase or new feature; one separate C commit per fix; if a preserved patch exists on origin/s557-repair-stack-quarantined use it only after confirming it fixes the reproduced failure; fixes in sibling repos land in umbrella/<repo> then bashy pin — present the diff for manager review before merging; do not push. Never touch $HOME/projects/poc/dhnt. Commit trailers Sprint: #561, Story: <this>, Story-ID: <id>; BASHY_AGENT = you. Deliver a short docs/s561/<story>-results.md with red/green evidence. Scope (A2 §4.1): F2 internal/agentos TestCommandDocSourceNames (extra 'desk'), F3 internal/agentos TestRunSignaledIsNonLossy (SIGKILL not signaled on Darwin), F4 internal/agentos TestSrcPinsMatchLinkedModules (yoke pin metadata; preserved hunk 8f20a0f8), F5 tools/bash53suite TestBashPPGateInputFixtureGetsExplicitSelector/0 status=TIME. F1 cmd/bashy TestYAMLAgentFenceCompilerRunner: setup (missing sibling ycode workspace) — record the prerequisite, no code. For each: reproduce, classify (product | test-env | metadata), fix or record. Gate: GOWORK=off go test ./... green except documented setup prerequisites.
