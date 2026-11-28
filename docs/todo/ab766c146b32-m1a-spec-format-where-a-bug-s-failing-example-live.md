---
id: ab766c146b32
kind: doc
title: 'M1a Spec format: where a bug''s failing example lives and how a known issue links to its fix'
seq: 3331
status: done
priority: p0
labels:
    - shapeware
created: 2026-11-28T06:50:09.54013Z
assignee: claude-opus5.5-s741
sprint: 742
sprint_id: 7dde34d4-f382-5aba-8ff5-071c5e3548b8
sprint_title: 'Shaping: bugs and issues become spec changes (issue-to-spec support)'
closed: 2026-11-28T08:31:21.424612Z
closed_by: claude-opus5.5-s741
---

Revives the minimal part of 734 L0 fce48cc11515 (closed wontfix). Decide and amend umbrella docs/shapeware-spec-v0.md 4.2, 11, 12: a bug's failing example is a tbd feature carrying resolves: KI-n (so the release class is not blocked); a known issue gains fixed-by: <feature or version>; a tbd feature that resolves a KI must have acceptance lines. Teach conform spec check the new keys. Verification: form, notepad and classgo reports-weekly-summary still lint clean; owner reviews the amendment. Needed first by ClassGo sprint 741.
