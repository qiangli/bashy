---
id: f1ef4efce5e6
kind: chore
title: S319 resync bashy sibling pins to published sh and yoke heads
seq: 359
status: done
priority: p1
labels:
    - bashy
created: 2026-09-30T13:08:28.331942Z
assignee: codex-gpt6-sol
sprint: 319
sprint_id: 8a7c3136-c235-5e11-933f-2cef714109b6
sprint_title: 'Bash# interpreted compiler packages: residual roots after Sprint 281 (R45)'
closed: 2026-09-30T13:26:46.642543Z
closed_by: codex-gpt6-sol
---

After Sprint 110 froze bashy 2db80ed and published sh 831b6b2d, Sprint 319 published sh d0f82b7f for types2 TestValuesInfo and yoke eabd672 includes the accepted 6ffc37c cleanup fix plus later docs. Update bashy .sibling-pins sh and yoke only to the exact current sibling HEADs, verify all pin entries against the umbrella, run relevant bashy gate(s), commit/push inside bashy with Sprint/Story trailers, then bump the umbrella bashy pin without sweeping unrelated working-tree changes. Preserve Sprint 110 frozen candidate; coordinate with its conductor.
