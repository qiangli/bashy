---
id: afc8ab8bd448
kind: feature
title: 'S524.2b-r bashy: rework account wiring (builtin import, both yoke pins, docs test)'
seq: 2830
status: done
priority: p1
labels:
    - accounts
created: 2026-10-29T09:38:13.83021Z
weave: 334
assignee: claude-sonnet5.5
sprint: 524
sprint_id: 9c8c2767-6917-5a65-bb86-481e65ce919a
sprint_title: Bashy account inventory, operational selection, and app summary
closed: 2026-10-29T11:43:54.516285Z
closed_by: s524-manager
---

REWORK of S524.2b (part of S524.2, umbrella story 0f9695efe8d8, Sprint 524): wire `bashy account`. A previous run's work is preserved in THIS repo at refs/salvage/s524-2b-332 (2 commits: ea85d450 yoke pin bump, e29718b3 wiring). Start by cherry-picking e29718b3 ONLY (unchanged; keep its message), then redo the pin and add fixes in NEW commits:
1. Pin: bump BOTH github.com/qiangli/yoke AND the nested module github.com/qiangli/yoke/pkg/provider to yoke main 96e0a61f (GOWORK=off go get github.com/qiangli/yoke@96e0a61f github.com/qiangli/yoke/pkg/provider@96e0a61f; go mod tidy) in commit `sync: bump yoke + yoke/pkg/provider pins (S524.2 accounts)`. If tidy wants other sibling bumps, keep them minimal and list them in the commit body.
2. Dispatch must link the built-in adapters: add a blank import `_ "github.com/qiangli/yoke/pkg/accounts/builtin"` where the account dispatch lives (like yoke's cmd/yoke/main.go).
3. Fix the regression: internal/agentos/commanddocs_test.go (~:25 hard-coded list) must include "account" for the new skills/bashy/commands/account.md.
4. Add a test: with BASHY_HOME at a temp dir and no provider env, `account discover --dry-run --json` (in-process) lists adapters cloudbox, llm, digitalocean, aws (e.g. via schema/adapters output or discover report) and exits 0; secret sentinel absent.
Gates: GOWORK=off go test ./internal/agentos/ -count=1 (the FULL package, not a -run subset), GOWORK=off go vet ./internal/agentos/, GOWORK=off go build ./..., GOOS=windows GOWORK=off go build ./... . Disk: Dragon is low — do NOT create per-run GOCACHE copies; use the default Go cache. Do NOT push. Trailers on new commits: Sprint: #524, Story: S524.2, Story-ID: 0f9695efe8d8 (plus this bashy story's full 12-hex id).
