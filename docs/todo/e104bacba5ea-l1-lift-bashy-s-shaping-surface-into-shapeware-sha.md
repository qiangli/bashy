---
id: e104bacba5ea
kind: task
title: L1 lift bashy's shaping surface into shapeware/shape/
seq: 3328
status: doing
priority: p1
created: 2026-11-27T16:53:33.234778Z
assignee: muse-wonder
sprint: 737
sprint_id: c5dc4786-3303-5f7b-8f04-1207027a550f
sprint_title: 'Lift bashy, outpost and cloudbox into shapeware (first surfaces, beacon: 2027 positioning)'
closed_by: claude-opus5.5
---

REDO (conductor, 2026-11-27): the shapeware/shape/ contract on main asserts only refusals (all 15 assertions target an unregistered app). Keep the refusal assertions; add the happy path end to end in a fresh HOME with a small fixture app the contract registers itself (bashy app add on a static folder under examples/): wish --from WORKSPACE (needs no agent), verify, preview, approve, status, history, show, diff, undo, restore, and scope set/ls granting a circle. A feature is verified only where an assertion proves it. Add a Beacon map section to SKILL.md (see the outpost lift shapeware/member-access/SKILL.md for the shape) and the standard gate targets check, cert, mutate, alone (umbrella docs/shapeware-spec-v0.md §5b). You may run the installed bashy against the fixture in a throwaway HOME to capture real outputs; conform spec check must be clean. The conductor runs cert, mutate and alone on the gate host. Commit with the Sprint/Story/Story-ID trailers and submit with: bashy sprint submit 737 e104bacba5ea -m EVIDENCE.
