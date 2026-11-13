---
id: f7be102864d2
kind: feature
title: 'bashy: generic read-only drift check for superprojects/submodules, wired into sprint checkpoint/stop/end and build evidence'
seq: 3229
status: todo
priority: p1
labels:
    - sprint
    - pins
    - integration
created: 2026-11-13T11:21:07.156281Z
---

Problem: delivered work keeps getting left out of builds — pins lagging pushed fixes, checkouts behind their pins (committing would roll the pin back), story files/todos uncommitted, commits unpushed, submitted weave runs unpulled — so gates and investigations run on stale trees and chase phantom issues (observed repeatedly; Sprints 626/627, 2026-11-12/13). The dhnt umbrella now has script/sync.sh --check (read-only; umbrella todo c6c8178f) and the conductor skill requires a drift check on every checkpoint and before stop/end (bashy 03b7fe3b), but that is per-repo shell and relies on the conductor remembering.
Want: a generic, provider-agnostic verb (e.g. `bashy sync check [--json]`) for any git superproject: per submodule pin vs remote default branch (behind/ahead), checkout vs pin (behind = never commit the gitlink; ahead = bump forward once pushed), unpushed commits, uncommitted tracked changes (attributed to the owning sprint/story when the path is a story file), submitted-but-unpulled weave runs; exit 0 in sync / 1 drift; JSON envelope. Wire it: `sprint checkpoint` shows it, `sprint stop/end` refuse on drift the sprint owns (explicit, logged override with a reason — escape hatch), builds/gates record the pin SHAs they used and flag checkout != pin. Start as an example/.bsh skill-script per the extend-bashy levels, graduate when proven.
Verify: unit tests over a fixture superproject covering each drift class + read-only proof (refs/porcelain unchanged); sprint end refuses with a named drift item and passes with the override.
