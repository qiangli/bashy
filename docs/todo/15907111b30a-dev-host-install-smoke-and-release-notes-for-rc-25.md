---
id: 15907111b30a
kind: task
title: Dev-host install, smoke and release notes for rc.25; promotion needs operator approval
seq: 3131
status: todo
priority: p1
labels:
    - release
created: 2026-11-06T20:10:28.175207Z
weave: 33
assignee: opencode-glm-5.3
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
resolution: fixed
closed_by: opencode-glm-5.3-g
---

After the candidate is gated: on the dev host run make install (canonical ~/.local/bin/bashy) from the frozen candidate and smoke the installed PATH binary (bashy --version, bashy sprint show, bashy agent list, bashy model, a dag smoke). Record installed path and results. Draft release notes using docs/public-claims-evidence.md checks (actual candidate tag, Bash 5.3, Go 1.27/go1.27.2). Propose promotion to the operator with the evidence; do not tag beyond -dev or publish a release without explicit operator approval. No v1.0.0 before official POSIX certification.

Added (sprint 580 plan, 2026-11-06): run the S1 shaping dogfood smoke (0a10de8d) on the installed binary; cut CHANGELOG [Unreleased] into an rc.25 section summarising the commits since v1.0.0-rc.24-dev by theme; write docs/releases/v1.0.0-rc.25.md that names software shaping and the local -> team/org -> upstream promotion path as the post-release extension route and links the K1 known-issues ledger (8d885a41). Promotion itself is R1 (0642298c).

Reopened (manager, 2026-11-06): the notes run (bashy#33, merged 258edbca) closed this story automatically, but only the CHANGELOG and release-notes draft is delivered. The dev-host make install, the smoke of the installed PATH binary, and the S1 dogfood remain, and they run on the frozen candidate.
