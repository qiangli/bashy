---
id: daf37e07c411
kind: task
title: Reconcile stale sibling pins in bashy and outpost before the rc.25 candidate
seq: 3129
status: done
priority: p0
labels:
    - release
    - deps
created: 2026-11-06T20:10:27.827767Z
weave: 34
assignee: codex-gpt6-astra
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
closed: 2026-11-07T01:41:08.565408Z
resolution: fixed
closed_by: codex-gpt6-astra-h
---

Problem: after the 2026-11-06 wrap-up, `bashy mod drift` (GOWORK=off) reports bashy pins behind sibling HEADs. Most drift predates the wrap-up; moving these pins pulls real, untested code into bashy, so it must be done deliberately with gates, not as a records-only bump.

Stale in bashy go.mod (pinned -> sibling HEAD at 2026-11-06):
- outpost (tool directive): 5b633ceb4995 -> 41efa72f1811
- yoke/external/otel, yoke/pkg/llmgw, yoke/pkg/oci, yoke/pkg/provider: 89df473c6035 -> a1f39ef256ab (root yoke already at a1f39ef2; nested modules lag)
- coreutils: 601546e19e1a -> eb3f31c0df7d (also the src_mount.go srcModulePins entry)
- coreutils/third_party/goawk (replace): 211789fadf75 -> eb3f31c0df7d
- bashsharp: 97efb5cc2ceb -> 45ae8e2d0f1d
- gotreesitter (replace): f03173f827ed -> 9471d547a17c
- bonsai: d14bdab6ed94 -> 24dd0f47247e
- ycode, ycode/examples/genie: cycle edge (bashy <-> ycode), 9392338234d3 / aa0d91dd60d2 -> 5e1538ce6281; bumped by hand per CLAUDE.md.
In sync: sh (5ea4c4b2), root yoke (a1f39ef2), readline, filebrowser, gfy, ollama src, podman src.
Outpost's own go.mod also lags yoke (root 89df473c, pkg/provider 89df473c/63180e3f, llmgw/oci replaces 63180e3f); bumping bashy's outpost pin implies reconciling outpost first.

Procedure: producer before consumer. (1) For each sibling, review commits between pin and HEAD (git log PIN..HEAD) and record whether they are code or records. (2) Reconcile outpost's yoke pins in outpost, gate outpost (go test -short ./... on a test host), push. (3) In bashy run `bashy mod sync` (or targeted go get / go mod edit for replaces), update internal/agentos/src_mount.go srcModulePins for yoke/coreutils/sh, run `GOWORK=off go list -deps ./...`. (4) Decide ycode cycle bump explicitly. (5) Gates on a self-hosted test host, never the dev box: GOWORK=off make test, Windows cross-build + e2e dispatch, make test-bash-container. (6) Re-run `bashy mod drift`; acceptance = no stale entry except a documented, intentional one.

Verification: drift report attached to the story; gate receipts with exact source SHAs; umbrella gitlinks bumped.

Order (sprint 580 plan, 2026-11-06): run after P1 (a130d972) lands in yoke, so the single bashy pin bump carries the promotion-path change; outpost reconciles first. If P1 misses the Saturday-noon cut line, proceed without it.

## Evidence (manager, 2026-11-06)

Outpost (producer):
- Outpost yoke pins were reconciled to a1f39ef256ab at outpost 5be964ec.
- Windows test-host gate, `go test -short ./...` with GOWORK=off on go1.27.2 windows/amd64: candidate bd7d3be 54 ok / 5 FAIL; base 41efa72 52 ok / 7 FAIL. Every candidate failure is pre-existing (Windows file modes, path separators, no federation on Windows, a missing fb_archives tag). No regression.
- Outpost uses only yoke/pkg/shape/appserver from the later yoke commits, and a1f39ef2..c7ab755b does not touch it. Outpost staying on yoke a1f39ef2 is therefore an intentional, documented lag.

Bashy pins are reconciled at bashy a022b00e, merged as 6de4c87f. Commit ranges, counting a non-merge commit as code when it touches anything outside docs/*.md:

| sibling | pin -> target | commits | code | records |
|---|---|---|---|---|
| coreutils | 601546e19e1a -> 97e83ec7afdf | 3 | 1 | 1 |
| coreutils/third_party/goawk | 211789fadf75 -> 97e83ec7afdf | 14 | 9 | 4 |
| sh | 5ea4c4b23230 -> d2913001421b | 2 | 1 | 0 |
| bashsharp | 97efb5cc2ceb -> 45ae8e2d0f1d | 1 | 0 | 1 |
| gotreesitter | f03173f827ed -> 9471d547a17c | 6 | 4 | 2 |
| bonsai | d14bdab6ed94 -> 24dd0f47247e | 15 | 8 | 5 |
| outpost | 5b633ceb4995 -> 5be964ec09d6 | 16 | 13 | 2 |
| yoke (nested modules) | 89df473c6035 -> c7ab755b6bc9 | 93 | 75 | 15 |
| ycode (NOT bumped; cycle) | 9392338234d3 -> 5e1538ce6281 | 17 | 10 | 7 |

Results:
- `bashy mod drift` after the merge shows only the documented ycode/genie cycle edge. The ycode bump is deferred to a separately gated change.
- Quick dev-box checks: build, vet, the agentos shape/pins/import tests and the core-import ratchet are green.
- The full gates run on the frozen candidate on the Windows test host (freeze story 7f785ad19fc2).
