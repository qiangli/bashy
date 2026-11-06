---
id: daf37e07c411
kind: task
title: Reconcile stale sibling pins in bashy and outpost before the rc.25 candidate
seq: 3129
status: todo
priority: p0
labels:
    - release
    - deps
created: 2026-11-06T20:10:27.827767Z
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
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
