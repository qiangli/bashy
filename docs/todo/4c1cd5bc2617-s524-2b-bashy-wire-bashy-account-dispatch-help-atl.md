---
id: 4c1cd5bc2617
kind: feature
title: 'S524.2b bashy: wire bashy account dispatch, help, atlas/MCP, yoke pin'
seq: 2828
status: wontfix
priority: p1
labels:
    - accounts
created: 2026-10-29T09:02:17.940717Z
assignee: claude-sonnet5.5
sprint: 524
sprint_id: 9c8c2767-6917-5a65-bb86-481e65ce919a
sprint_title: Bashy account inventory, operational selection, and app summary
closed: 2026-10-29T16:07:53.763942Z
---

Part of S524.2 (umbrella story 0f9695efe8d8, Sprint 524), slice B (bashy): wire `bashy account` to the yoke command (yoke/pkg/accounts NewAccountCmd, on yoke main since 23f4bc74; a follow-up yoke slice S524.2c is changing refresh/registry internals but not the NewAccountCmd constructor).
1. Pin: this workspace builds with GOWORK=off against bashy go.mod. Bump the yoke requirement to yoke main (GOWORK=off go get github.com/qiangli/yoke@main, then go mod tidy) in its own commit `sync: bump yoke pin (S524.2 accounts)`. If other sibling pins must move, follow docs in this repo for mod sync; never vendor.
2. Wiring: add "account" to alwaysShimVerbs (internal/agentos/agentos.go ~:157); a dispatch case modelled on the `secret`/`secrets` arm (~:1375: construct the cobra cmd, setCobraArgs, dispatchExit); flag schema in internal/agentos/cobra_schema.go (~:146 cobraVerb); verbSynopsis in commands.go (~:593); help section in commands_sections.go (~:60); a command doc under skills/bashy/commands/ like its siblings. MCP comes from the yoke atlas entry automatically (mcp_verbs.go) — verify `account` appears there.
3. Tests: dispatch test that `bashy account schema --json` and `bashy account list --json` (with BASHY_HOME set to a temp dir) run in-process and exit 0; help/synopsis/atlas consistency tests that already exist in agentos must pass. Do NOT exec the test binary or bashySelfPath (it re-runs the test binary).
Gates: GOWORK=off go test ./internal/agentos/ -run 'Account|Dispatch|Synopsis|Atlas|Help|MCP' -count=1; GOWORK=off go vet ./internal/agentos/; GOWORK=off go build ./...; GOOS=windows GOWORK=off go build ./... . Placeholder names only. Do NOT push (bashy main is under a release hold). Trailers: Sprint: #524, Story: S524.2, Story-ID: 0f9695efe8d8 (plus this bashy story id).
