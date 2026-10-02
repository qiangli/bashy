---
id: 5c1e024d87ad
kind: task
title: S369.2 Grammar attribution regenerated against the fork; generator fails closed
seq: 397
status: todo
priority: p1
labels:
    - licensing
    - yoke
created: 2026-10-02T22:40:17.65082Z
sprint: 369
sprint_id: ac323bec-85c1-53d1-9796-b27703113450
sprint_title: Permissive-only grammar set and Java as a registered fence
---

Goal
yoke/THIRD_PARTY_GRAMMARS.md is the attribution for the 201 kept grammars. After S369.1 it must be regenerated against the fork and show an empty EXCLUDED section.

Contract
- scripts/grammar-licenses.sh exits non-zero when any grammar resolves to a non-permissive or UNKNOWN license (today it only refuses UNKNOWN). Keep the hand-verified override table for repos whose license lives only in package.json/Cargo.toml.
- Regenerate; commit the file; bashy/THIRD_PARTY_LICENSES.md finding 2 closes by pointing at it.

Acceptance
- Script green against the fork; EXCLUDED section empty; 201 rows; Sprint 350's SBOM gate can read this file as the grammar attribution.
