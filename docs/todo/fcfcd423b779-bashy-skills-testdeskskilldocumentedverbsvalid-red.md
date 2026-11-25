---
id: fcfcd423b779
kind: bug
title: bashy/skills TestDeskSkillDocumentedVerbsValid red on main (notepad --help=mcp exits 1), pre-existing at f739916a
seq: 3309
status: todo
priority: p2
labels:
    - test
    - pre-existing
created: 2026-11-25T06:49:11.471362Z
sprint: 713
sprint_id: 867b4689-21ed-53db-93f9-b2a926b28de6
sprint_title: 'Known reds triage after train 1: decide fix or won''t do for each pre-existing test red'
---

Seen 2026-11-25 by the Sprint 699 pin bump gate (gate host WSL lane 2, root, Go 1.27.4, GOWORK=off, go test -short -count=1). managed_skills_test.go:112: failed to run --help=mcp for notepad: exit status 1. Identical at baseline f739916a and at candidate d709296a, so not caused by the pin bump. Root cause not yet identified (venue vs product): compare with the last green run of this test, check whether the notepad desk tool builds/registers in a clean clone as root, then classify. Carried into the 699 gate as a recorded failure, never a pass.
