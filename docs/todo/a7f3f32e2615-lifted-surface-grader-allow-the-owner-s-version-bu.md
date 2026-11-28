---
id: a7f3f32e2615
kind: enhancement
title: 'Lifted-surface grader: allow the owner''s version bump in --spec-change and compare against a clean installed tree'
seq: 3339
status: todo
priority: p2
labels:
    - shapeware
created: 2026-11-28T10:53:12.518639Z
---

From the first shaped ClassGo fix (sprint 741 B1, 2026-11-28). (1) candidate.bsh --spec-change refuses any SKILL.md change outside Known issues, so the patch bump that spec 10 requires for a fixed known issue (metadata.version) cannot ride with the candidate; allow exactly a metadata.version change in --spec-change mode (and check it is a semver bump), or define that the owner bumps it at approval. (2) The grader compares the copy against the installed implementation's working tree, so uncommitted files there (tracker cards, a test-written git-ignored config.json) make every candidate fail; compare against a clean export of the installed HEAD (git archive) plus the declared build outputs, and say so in spec 4.3b. Also: surfaces that copied the grader before 13e33577 still carry the printf - bug; index.bsh could check each surface's candidate.bsh against the canonical one.
