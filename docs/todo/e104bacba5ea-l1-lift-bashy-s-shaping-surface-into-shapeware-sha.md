---
id: e104bacba5ea
kind: task
title: L1 lift bashy's shaping surface into shapeware/shape/
seq: 3328
status: done
priority: p1
created: 2026-11-27T16:53:33.234778Z
assignee: muse-wonder
sprint: 737
sprint_id: c5dc4786-3303-5f7b-8f04-1207027a550f
sprint_title: 'Lift bashy, outpost and cloudbox into shapeware (first surfaces, beacon: 2027 positioning)'
closed: 2026-11-27T17:19:54.452166Z
closed_by: claude-opus5.5
---

Lift bashy's shaping surface. Surface: bashy shape (wish, verify, preview, approve, undo, restore, history, scope, status) and the site MCP shaping tools, as they behave on one host. Folder: shapeware/shape/ (bashy already carries shapeware/index.bsh). Reference runs to reuse: tests/group-shaping-e2e and tests/field-loop-e2e (fakes, gate host).

Method: kb runbook shapeware-lift-app (bashy kb/defaults; read it first) and the format shapeware-spec-v0 §1, §4.3a and Appendix C plus the workflow standard (umbrella docs/). Lift ONE surface into shapeware/<surface>/ as a lifted surface (§4.3a): SKILL.md with the 'what it is' line and metadata.implementation / metadata.reference-commit, the two-way source step in dag.md, Build in a work copy of the repository, features.yaml, CONTRACT.md (rules first, then assertions from real runs), examples/, known issues and tbd features. Copy shapeware/index.bsh verbatim from bashy if the repo has none; add the discovery line to CLAUDE.md/AGENTS.md and .shapeware/ to .gitignore.

Acceptance (all at the delivery commit): (1) bashy conform spec check shapeware/<surface> reports nothing and bashy --bashsharp shapeware/index.bsh check passes; (2) conform cert 3 runs, none noisy, and conform mutate kills every mutant or each survivor is recorded with its reason, run on the gate host with sandbox fakes, raw logs kept; (3) the folder stands alone: copied outside any checkout, an agent that has not seen the repository reaches a passing Check from the folder's own text and lifecycle; (4) the known issues listed below are in the spec; (5) the diff touches only the spec folder, the discovery line and .gitignore. No product code changes: a defect found becomes a known issue plus a todo. Builds and tests only on the gate host, never the dev box. Commits carry Sprint/Story/Story-ID and Agent trailers.

Known issues the spec must carry: the group shaping loop is proven only in a sandbox with a stub agent; app data across a change and a restore is not asserted; the site MCP has shape_undo but no restore of a named version; conform is not called by shape verify.

Code inspected 2026-11-27: internal/agentos/mcp_site.go, shape_local.go, shape_remote.go; tests/group-shaping-e2e/sandbox.py, tests/field-loop-e2e/run.py; examples/desk/form and notepad; yoke pkg/shape/service.go, slot/slot.go, program_runtime.go.
