---
id: 5c245daf022e
kind: feature
title: 'Hosted-CI quickstart: pinned checksum-verified bashy install then bashy dag TARGET, green on Linux/macOS/Windows runners'
seq: 1936
status: todo
priority: p1
labels:
    - release
    - dag
    - ci
created: 2026-10-17T15:14:11.182068Z
---

Carved 2026-10-17 from post-1.0 story c3e3127810d5 (S367.4, Sprint 403 closed) for the v1.0.0 launch bar (Sprint 128 p5d CI smoke test; p5b release CI green on three OSes).

Scope
- One 4-6 line GitHub Actions snippet: install a pinned, checksum-verified bashy release, then run bashy dag TARGET.
- Reuse the install steps already in .github/workflows/install-matrix.yml and quickstart-scratch.yml. No new reusable action, no per-language CI graphs.
- Prove it once on the bashy repo on Linux, macOS and Windows hosted runners (a workflow job or matrix leg that runs the snippet verbatim).
- Publish the snippet in README and docs/dag.md (a short how-to, not positioning; see docs/positioning-decision-2026-07-25.md in the umbrella). No real hostnames, user ids or addresses.

NOT this story: the full dag-as-front-door doc section, fence methods, foreign includes (still in c3e3127810d5, post-1.0).

Acceptance: the published snippet, copied verbatim into a workflow, is green on all three OS runners against the release candidate; checksum verification fails closed on a tampered archive (red/green); run links recorded in the Sprint 407 evidence record.
