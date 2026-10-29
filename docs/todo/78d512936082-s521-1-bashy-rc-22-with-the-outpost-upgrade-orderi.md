---
id: 78d512936082
kind: task
title: 'S521.1 bashy rc.22 with the outpost upgrade-ordering fix (outpost cd8b98c4): pin + fast loop + promote'
seq: 2753
status: done
priority: p0
created: 2026-10-28T05:56:21.244131Z
assignee: s521-manager
sprint: 521
sprint_id: 8215f52e-1496-5a0f-b6f6-729bf3056119
sprint_title: 'Fleet upgrade fix release + ivy deploy: ship outpost upgrade-ordering fix (cd8b98c4) as bashy rc.22, move every host incl. ivy (lern, lj2ivy) onto it once, then promotions auto-flow'
closed: 2026-10-29T10:56:14.165447Z
closed_by: s521-manager
---

Move bashy's outpost pin to cd8b98c4 (or newer main) and release rc.22 through kb:release-bashy Fast loop rules 1-11 (local gate, release-preflight, CI, release-qa, POSIX reuse check, tier smoke after assets, promote with pinned verdict URL). Evidence docs/sprint-521/release-v1.0.0-rc.22.md. Handoff: docs/handoff-2026-10-28-sprint-521-fleet-fix-ivy.md (read it first).
