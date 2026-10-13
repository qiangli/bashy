---
id: 85ea5a704276
kind: task
title: Install reviewed Ollama runner fork tree and run hosted release gates
seq: 1924
status: wontfix
priority: p1
created: 2026-10-13T17:18:20.371231Z
assignee: s436-codex-manager
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T18:41:52.252125Z
---

After compiled-input gate review, install exact generated builds/ollama/fork tree atop qiangli/ollama bashy-build branch without force or loss of prior S434 commit/tag; push reviewed branch commit with Sprint/Story trailers, then immutable bashy-v0.21.2-N tag to trigger five native build, protocol/no-egress, audit/SBOM and signing jobs. Verify every hosted artifact, identity, issuer, sums, attestations and target pin before claiming release. Preserve failed runs as evidence and do not sign/accept bypasses. No Dragon heavy builds or containers.

Merged into Sprint 436 parent story 732cbb22d353 (signed embedded runner shipped). Preserve existing commits, fork tags, failed-run evidence and this checklist under the parent; this child is not independently accepted. Required release targets are Linux amd64/arm64, Darwin arm64, and Windows amd64; Darwin amd64 is optional. The operator restored Windows to Sprint 436 scope.
