---
id: 85ea5a704276
kind: task
title: Install reviewed Ollama runner fork tree and run hosted release gates
seq: 1924
status: assigned
priority: p1
created: 2026-10-13T17:18:20.371231Z
assignee: s436-codex-manager
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
---

After compiled-input gate review, install exact generated builds/ollama/fork tree atop qiangli/ollama bashy-build branch without force or loss of prior S434 commit/tag; push reviewed branch commit with Sprint/Story trailers, then immutable bashy-v0.21.2-N tag to trigger five native build, protocol/no-egress, audit/SBOM and signing jobs. Verify every hosted artifact, identity, issuer, sums, attestations and target pin before claiming release. Preserve failed runs as evidence and do not sign/accept bypasses. No Dragon heavy builds or containers.
