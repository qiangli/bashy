---
id: fd7a7738a84b
kind: bug
title: 'viewer: source rebuild fails on unpublished desk modules; status template cannot read the sprint card from sprint show --json'
seq: 3319
status: todo
priority: p2
created: 2026-11-25T19:29:20.898891Z
sprint: 721
sprint_id: 69fc2c51-8faf-58e7-83ac-f27007837ae7
sprint_title: 'Shapeware pilot on form: spec v0.1 layout, lifecycle, one-step launch'
---

Seen 2026-11-25 (Sprint 721, putting the sprint status in bashy viewer). (1) bashy viewer add rebuilt from source and failed: 'reading examples/desk/chart/go.mod at revision examples/desk/chart/v0.1.3: unknown revision' (local Go module proxy not published). Same class as 64ef2626 addendum 1; the g5 fix (publish desk modules before a stale build) covered form.bsh and chart.bsh only, not viewer.bsh (or notepad/page). Workaround: run examples/desk/register-modules.sh. (2) append --template status with the sprint omitted failed 'sprint card not found in sprint show output': bashy sprint show N --json now returns an adapt envelope {schema_version, command, status, result:{sprint, goal_progress, ...}}, and the x-source: sprint filler does not look under result. Workaround: pass the sprint object in --data. (3) Observation, not viewer: goal_progress read 3/8 because story status is derived from the shared bashy checkout at an older commit. Root cause first for (1) and (2) before fixing.
