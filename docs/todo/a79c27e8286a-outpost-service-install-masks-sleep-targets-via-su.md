---
id: a79c27e8286a
kind: task
title: outpost service install masks sleep targets via sudo on Linux
seq: 3189
status: todo
created: 2026-11-08T01:46:28.638404Z
sprint: 591
sprint_id: dc240e08-28b0-5592-85a1-15e3d650c716
sprint_title: 'README as agent bootstrap: human hook + agent skill, one consent gate'
---

outpost service install first runs systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target (system-wide, sudo). Make the power step opt-in or prompt-free-skippable, and state it in --help; README option b discloses it today.
