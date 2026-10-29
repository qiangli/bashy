---
id: c67905608f9d
kind: refactor
title: 'S525.A1b bashy: agentos onto yoke pkg/unit + contract call accounting + yoke pin'
seq: 2829
status: done
priority: p1
created: 2026-10-29T09:29:38.305805Z
weave: 333
assignee: claude-opus5.5
sprint: 525
sprint_id: 9a9a4443-02c8-53a3-b0bd-96bd67980643
sprint_title: 'Software Shaping foundation: verified personal features and same-host promotion'
closed: 2026-10-29T10:18:26.859011Z
resolution: fixed
closed_by: claude-opus5.5-u
---

Part of S525.A1 (umbrella b976805c404f) + A2 (6bd2374295a2). Rides rc.23 (rc.22-dev tagged).
Scope:
- Bump yoke require to github.com/qiangli/yoke main c939ec6f (pushed; pseudo-version via go get), go mod tidy.
- Migrate bashy agentos consumers to the one unit model in yoke pkg/unit (fence extractor, identity, contract reader): internal/agentos/{inspect_actions,commanddocs,attest,polyglot_rows,skills_bashsharp,context}.go — drop local fence/identity/contract parsing duplicates. Prose-only command docs (skills/bashy/commands/*.dhnt) stay discovery-only.
- Call accounting: in the contract decorator path (internal/agentos/contracts.go requireDecorator/ensureDecorator, before the body runs) call yoke pkg/shape/verify.RecordCall(<function name>) — no-op unless BASHY_SHAPE_CALLLOG is set.
- runSkillBashSharp installs dag.CapExecHandler()/JobCarrier like the dag seam, if not already.
Acceptance (one focused red/green test): go test ./internal/agentos -run 'TestS525A1|TestContractCallLog' — paired dhnt/Bash# faces => same capability key via pkg/unit; with BASHY_SHAPE_CALLLOG set, invoking an @ensure-decorated fn appends its name, an uncalled one does not; plus go build ./... and go test ./internal/agentos green. Do NOT run make test-bash on this host (heavy). Never exec bashySelfPath() from tests. Behavior-only docs/commits, no hostnames. Trailers Sprint: #525 / Story: #<seq> / Story-ID.
