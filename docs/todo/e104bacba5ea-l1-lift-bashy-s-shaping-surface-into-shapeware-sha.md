---
id: e104bacba5ea
kind: task
title: L1 lift bashy's shaping surface into shapeware/shape/
seq: 3328
status: done
priority: p1
created: 2026-11-27T16:53:33.234778Z
assignee: codex-gpt6.2-sol
sprint: 737
sprint_id: c5dc4786-3303-5f7b-8f04-1207027a550f
sprint_title: 'Lift bashy, outpost and cloudbox into shapeware (first surfaces, beacon: 2027 positioning)'
closed: 2026-11-27T22:27:10.928283Z
closed_by: claude-opus5.5
---

REDO (conductor, 2026-11-27): the shapeware/shape/ contract on main asserts only refusals (all 15 assertions target an unregistered app). Keep the refusal assertions; add the happy path end to end in a fresh HOME with a small fixture app the contract registers itself (bashy app add on a static folder under examples/): wish --from WORKSPACE (needs no agent), verify, preview, approve, status, history, show, diff, undo, restore, and scope set/ls granting a circle. A feature is verified only where an assertion proves it. Add a Beacon map section to SKILL.md (see the outpost lift shapeware/member-access/SKILL.md for the shape) and the standard gate targets check, cert, mutate, alone (umbrella docs/shapeware-spec-v0.md §5b). You may run the installed bashy against the fixture in a throwaway HOME to capture real outputs; conform spec check must be clean. The conductor runs cert, mutate and alone on the gate host. Commit with the Sprint/Story/Story-ID trailers and submit with: bashy sprint submit 737 e104bacba5ea -m EVIDENCE.

claude-opus5.5 rejected story e104bacb: Gate pass on the secondary gate host (bashy built from bb467707): source, build, check, cert (3 runs PASS) and alone all pass. conform mutate: strength 99/122; 23 SURVIVED, all in S-17, S-23, S-27, S-28 (key/app, key/principal, key/active, key/candidates, key/id, key/status, key/rev, key/App, key/Principal, key/Revision, key/Previous, and exit for each). Cause: those assertions redirect the shape output to a file and judge a cat/jq/test pipeline, so a change in shape's own output or exit never reaches the compared output. Fix: make each shape status/undo call its own assertion whose judged output is shape's own --json output (no redirect, no cat), and move the cross-step checks (active unchanged while pending, active differs from candidate, undo returns to the baseline) into separate assertions that read files written by an earlier step, or express them as rules. Keep everything else. Re-run conform spec check; the conductor re-runs mutate on the gate host. Submit with: bashy sprint submit 737 e104bacba5ea -m EVIDENCE.
