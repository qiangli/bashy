---
id: fcfcd423b779
kind: bug
title: bashy/skills TestDeskSkillDocumentedVerbsValid red on main (notepad --help=mcp exits 1), pre-existing at f739916a
seq: 3309
status: wontfix
priority: p2
labels:
    - test
    - pre-existing
created: 2026-11-25T06:49:11.471362Z
sprint: 724
sprint_id: 4c6a8940-8673-582a-adef-d7665a30d524
sprint_title: 'Consolidated follow-ups after train 1 (supersedes 711, 713, 714, 715, 718, 720): owner-sharing and A/O/C sandbox e2e, site MCP and site attach, deferred integration runs, known reds, MCP 2026-07-28'
closed: 2026-11-26T11:16:26.90963Z
---

Seen 2026-11-25 by the Sprint 699 pin bump gate (gate host WSL lane 2, root, Go 1.27.4, GOWORK=off, go test -short -count=1). managed_skills_test.go:112: failed to run --help=mcp for notepad: exit status 1. Identical at baseline f739916a and at candidate d709296a, so not caused by the pin bump. Root cause not yet identified (venue vs product): compare with the last green run of this test, check whether the notepad desk tool builds/registers in a clean clone as root, then classify. Carried into the 699 gate as a recorded failure, never a pass.
