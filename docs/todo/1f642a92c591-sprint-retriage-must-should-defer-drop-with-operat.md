---
id: 1f642a92c591
kind: feature
title: 'sprint retriage: must/should/defer/drop with operator approval for deferral'
seq: 3115
status: todo
priority: p1
labels:
    - sprint
    - triage
created: 2026-11-06T17:13:23.420718Z
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

MVP: (1) Stories added after sprint start are marked untriaged; sprint show prints 'planned N -> now M (U untriaged)'. (2) bashy sprint retriage ID STORY --as must|should|defer|drop -m why records disposition in frontmatter + a decision entry. must = blocks acceptance, regression of a previously green gate, safety/data loss, or prerequisite of a must. (3) defer/drop are pending until bashy sprint approve ID STORY --by OPERATOR, unless a card directive pre-approves them (see directives story); the proposer cannot approve. (4) Approved defer with --to N moves the story (existing todo edit --sprint path) and records carried_from. Out of MVP: auto thresholds, new-sprint creation, goal rewriting. Verification: unit tests per disposition, self-approval refusal, pre-approval match, carry. Baseline: S562 assessment (operator-started Codex manager: 0/69 self-initiated turns, 4-9.5h reaction lag, 18 submissions pending 29-38h, 0 rejects, 5 self-accepts, 10 planned stories grew to 41).
