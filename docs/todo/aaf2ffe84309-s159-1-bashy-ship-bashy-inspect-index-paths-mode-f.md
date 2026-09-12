---
id: aaf2ffe84309
kind: task
title: 'S159.1 bashy: ship bashy inspect (index, paths, mode); fold doctor/context/audit behind hidden aliases'
seq: 257
status: todo
priority: p0
created: 2026-09-12T19:15:05.106924Z
sprint: 159
---

bashy half of S159.1 (umbrella story f1d46cf5c54f; design docs/bashy-inspect-design.md in the umbrella, PRIVATE).

New verb bashy inspect (internal/agentos/inspect.go): the index (every question about bashy and the verb that answers it, checked against the live catalog so it can never name a verb that does not exist), paths (the resource map: 31 rows, every path from the owning package's accessor, scope-resolved todo/repo-graph rows for the current cwd, secrets by path+existence only), and mode (every middleware gate with the signal that decided it, calling the real deciders; first caller of fleet.MarkerEnvs). doctor/context/audit fold behind it as aspects with their bodies unchanged; the old names become hidden aliases (hiddenFrontDoorVerbs) and still dispatch byte-identically. BASHY_AGENT_MANIFEST, context.recommended_commands, help, the bashy skill and the live docs are re-pointed; historical retros/results are left as written.

Gate: TestInspectIndexNamesOnlyAtlasVerbs, TestInspectNoPathLiterals, TestInspectModeReportsDecisions (table-driven over the deciding signals, agreeing with the real deciders), TestInspectPathsDeterministic, TestInspectPathsContract, TestInspectFoldedAspectsDispatch, TestAtlasCoversEveryCommand; go list -deps ./cmd/bash links nothing from agentos/coreutils; CGO_ENABLED=0 windows/amd64 cross-build; script/bashy-inspect-eval/run.sh all four checks PASS in shipped mode.
