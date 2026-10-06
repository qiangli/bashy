---
id: 62e0290fec9f
kind: chore
title: S369.3 Offer the grammar exclusion upstream so the fork can retire
seq: 398
status: assigned
priority: p3
labels:
    - licensing
created: 2026-10-02T22:40:18.333949Z
weave: 16
assignee: claude-sonnet5.5
sprint: 369
sprint_id: ac323bec-85c1-53d1-9796-b27703113450
sprint_title: Permissive-only grammar set and Java as a registered fence
---

Goal
A build tag (or a generated manifest flag) in odvcencio/gotreesitter that lets a consumer exclude named grammars from the embedded set, so qiangli/gotreesitter can go back to a pin on upstream.

Contract
- One upstream PR: the tag, the five grammars marked non-permissive in languages.manifest, README line. No other change.
- If merged: drop the fork, pin upstream, keep S369.2's check as the guard.

Acceptance
- PR opened and linked here; fork README states it exists only until the PR lands.
