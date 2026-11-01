---
id: 17bccfc3f954
kind: chore
title: S557 align standalone release candidate with validated producer revisions
seq: 3037
status: assigned
priority: p1
created: 2026-11-01T17:24:07.911637Z
assignee: s557-manager
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
---

After native fixes, update Bashy versioned sibling pins to reviewed published producer revisions so standalone artifact includes repaired TSX/Bonsai/provider/Ycode behavior and exact sh/core provenance. Use existing mod sync with scoped producer selection, preserve cycle exceptions, update srcModulePins full SHA metadata, validate standalone graph and targeted metadata tests, then current official/full gates. Do not publish release tag.
