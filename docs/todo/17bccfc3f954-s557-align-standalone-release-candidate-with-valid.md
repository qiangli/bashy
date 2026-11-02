---
id: 17bccfc3f954
kind: chore
title: S557 align standalone release candidate with validated producer revisions
seq: 3037
status: wontfix
priority: p1
labels:
    - superseded
created: 2026-11-01T17:24:07.911637Z
assignee: s557-manager
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
closed: 2026-11-02T05:32:08.417684Z
---

After native fixes, update Bashy versioned sibling pins to reviewed published producer revisions so standalone artifact includes repaired TSX/Bonsai/provider/Ycode behavior and exact sh/core provenance. Use existing mod sync with scoped producer selection, preserve cycle exceptions, update srcModulePins full SHA metadata, validate standalone graph and targeted metadata tests, then current official/full gates. Do not publish release tag.

## Superseded by consolidation — 2026-11-02

Operator requested consolidation of implementation repairs into acceptance owners. Scope, original acceptance, unresolved work and evidence transfer to public umbrella #4, story `92b4bc0b7791`. This record is closed by decision with label `superseded`; it is NOT a delivery acceptance, test pass, waiver or feature retirement. The parent remains open. Continue corrections under that parent rather than creating another repair story. Original content above is retained unchanged.
