---
id: a130d972faf3
kind: feature
title: 'P1 Promotion rod: shape outgoing/push and incoming/pull between local, team/org and upstream'
seq: 3147
status: assigned
priority: p0
labels:
    - release
    - shaping
created: 2026-11-06T21:49:59.999939Z
assignee: codex-gpt6-sol
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
---

Problem: rc.25 ships as a shapeable minimum; post-release features and fixes move between three tiers: the local host, the team/org, and the upstream bashy repo. Humans and agents (agents first) need a "rod": one easy query for what is worth promoting or pulling, and one command each way, like git diff/push/pull. Today Service.Submit (yoke/pkg/shape/submit.go) opens only a LOCAL src PR plus a weave; nothing pushes, nothing reports incoming changes, and src offer/adopt refuses CLI/service kinds.

MVP (KISS), all under bashy shape, --json with a stable versioned schema on every verb, plain text for humans, and exit codes an agent can branch on (0 = nothing pending, 1 = error, 3 = changes pending):
1. Tiers: `shape remote` shows/sets two named remotes, org (team/org remote) and upstream (default: the original bashy repo). It reuses the src upstream store (yoke/pkg/src/upstream.go), widened from one URL per repo to named tiers.
2. Outgoing (query + diff): `shape outgoing [--to org|upstream] [--diff]` lists promotion-worthy local changes: approved and verified shapes, with their provenance record, that are not yet in the target. Presence is decided by patch-id, the same way as git cherry. Already promoted to org but not yet upstream shows as pending for upstream.
3. Push: `shape push --to org|upstream [<id>...]`. To org, it pushes the shape-submit/<id> branch with go-git, reusing the yoke/pkg/src/core_repair.go push. To upstream, it pushes to a fork/branch and runs `gh pr create`, with the body from the Submission record (Shape/Shaped-By trailers, verifier evidence path). Without gh it prints the branch and compare URL.
4. Incoming (query): `shape incoming [--from org|upstream]` lists new changes available. It fetches the remote, then reports shape-submit branches and commits not in the local base, plus newer bashy releases (tags) than the installed one.
5. Pull: `shape pull --from org|upstream [<id>|<tag>]`. A source change goes through the existing gated lifecycle: wish --from <fetched checkout> -> verify -> preview -> approve. It runs from the side slot and stays undoable with shape undo. A newer release goes to the existing install/upgrade path, not a rebuild.
6. Discovery: shape status reports a one-line summary, "N outgoing (org/upstream), M incoming", so agents see it without extra calls.

Order: lands in yoke first, then the bashy CLI wiring, so the single pin-reconciliation bump (daf37e07c411) carries it. Cut line: if outgoing/push are not merged by Saturday noon, freeze without P1; it becomes the first shaped change after release and the K1 ledger records it. Incoming/pull may follow as a shaped change if outgoing/push make the cut and they do not.

Deferred: kind-aware artifact offer/adopt, team trust/approval policy for adopting others' changes, and the cross-platform shaping matrix (Sprint 579 H5/H7; boundary circles in Sprint 583).

Verification: unit tests for tier config, the patch-id presence check, the exit codes and the push/gh argument building; one recorded round trip between two hosts (by role) through a scratch org remote: A shapes -> outgoing shows it -> push --to org -> B incoming shows it -> B pull -> B shape undo; then push --to upstream opens a PR (or prints the compare URL without gh); GOWORK=off go test for yoke/pkg/shape, yoke/pkg/src and bashy internal/agentos shape tests.
