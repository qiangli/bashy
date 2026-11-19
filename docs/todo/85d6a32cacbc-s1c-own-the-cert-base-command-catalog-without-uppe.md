---
id: 85d6a32cacbc
kind: refactor
title: S1C Own the cert-base command catalog without upper-layer imports
seq: 3241
status: todo
priority: p0
labels:
    - superseded-plan
created: 2026-11-18T00:56:29.150374Z
sprint: 575
sprint_id: 65ce85f5-87c0-5863-bf55-62de86f5df36
sprint_title: 'Layer separation: Classic <- Bash# <- Yoke import-graph delineation'
---

Sprint575 S1 implementation slice, following source reviews recorded in umbrella .agents/s575/neutral-boundary-review.log and S1-complete-design.log. Read umbrella docs/plan-s575-master-execution.md, docs/s575-runtime-boundary.md and this repo CLAUDE.md. Goal: cert-base internal/core stops importing Yoke/Bashsharp through the command catalog, preserving existing command behavior except the explicitly required refusal of Bashsharp execution in a Bash-only profile. Full AgentOS Yoke catalog remains unchanged.

Place a private owned command-only mechanism under internal/core (child package as appropriate), copying existing command-relevant source and tests from frozen Yoke c9057f1e. Preserve record fields/defaults and complete commands CLI (bare/list/show/schema/retire/unretire/add/register/set/rm/edit/verify), shared/cached-org/local ring precedence, sparse overlay ACTUAL baseline behavior, aliases/shadows/warnings, schema/vocabulary/collision/applicability checks, local atomic writes/lifecycle, applet->registered command->PATH route, VSC_PROFILE=cert suppression, syntax probes, env/cwd/exit mapping and exec/script/download paths.

Manager scope decisions: preserve current actual overlay behavior (commands kind currently Overlay=false), even though help promises more; do NOT fix that pre-existing defect during this migration. Preserve the core profile's existing unwired claim guard default; do NOT invent live claim protection or import upper fleetkinds. Full profile already uses Yoke's catalog and must remain untouched. No new public plugin interfaces.

Carry only neutral acquisition helpers required by Command.Ensure: cached binary lookup FIRST, then platform SHA256 refusal before cold network; preserve pinned URL/GitHub resolution, cache/offline rules, verified raw/member/archive download, archive formats, atomic installation and executable policy. Keep source logic, errors and existing test assertions. Do not copy optional media/tool policy, telemetry, prune or agent registries. Prefer copied minimal existing helper closure to rewritten mechanisms; if preserving a helper requires adjacent neutral code, document exactly what was copied. No foundation/coreutils/Yoke/Bashsharp edits or lower pin changes.

For .bsh catalog records, preserve discovery/YAML precedence/cache/degraded warnings, explicitly report unsupported Bashsharp record instead of executing or pretending absent. Script default dialect bashpp must be refused at execution/probe; explicit dialect bash retains its existing route. This is authorized removal of Bashsharp from cert-base, not permission to drop YAML CRUD/download behavior. A small private execution seam is allowed if necessary, no broad adapter framework. S2 separately removes base main's blank Bashsharp import/CLI wiring; do not claim all cert-base edges gone here.

Implement internal/core/commands.go reference update plus new owned private package and directly necessary command-only dependencies. Preserve tests by copying relevant existing command/ring/lifecycle/overlay/binmgr cases, adapting only ownership references and explicit Bashsharp refusal expectations. Add focused missing-digest cold refusal, warm cache-before-digest and unsupported-language tests if absent. Do not weaken/delete/skip tests or touch source outside this slice. Do not modify go.mod versions just to pass failures; only direct-require promotions at existing resolved versions if necessary, record them. No KB.md/generated docs, no unrelated fixes, no root umbrella writes, pushes or branch switches.

ALL builds/tests/Go execution run only in manager's locked official image. Do not run go build/test/run locally. Static formatting/metadata inspection only; manager runs meaningful source comparison and official-image DAG gates. Commit isolated workspace with correct Sprint575/Story/Story-ID trailers. Report exact copied source closure, preserved quirks, final SHA and remaining checks. If dependency closure cannot fit this reviewed plan without broader redesign, stop and report concrete blocker instead of inventing APIs.
