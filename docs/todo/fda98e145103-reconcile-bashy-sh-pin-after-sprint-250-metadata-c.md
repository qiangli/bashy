---
id: fda98e145103
kind: chore
title: Reconcile Bashy sh pin after Sprint 250 metadata closure
seq: 338
status: todo
priority: p2
labels:
    - release
    - sh
created: 2026-09-24T02:50:21.63043Z
sprint: 266
sprint_id: 16738595-c072-5838-be1b-603932c6df8e
sprint_title: Bashy shell, Coreutils, and BashSharp follow-up after v0.28.0
---

After v0.28.0 was tagged at tested Bashy 7416f0b, sh master advanced only through Sprint 250 closure metadata to 4d218aba. Bashy main cannot push its own closure metadata while .sibling-pins still names older sh 0f7f9022. Advance only the Bashy sh sibling pin to public sh master 4d218aba, leave v0.28.0 tags and published assets fixed at tested bytes, and verify clean non-force push plus pin equality. This is post-tag default-head hygiene, not a product change.
