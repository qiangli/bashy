---
id: f3263fbd0cad
kind: bug
title: 'S524.7c bashy: fix connect --account fixture for identity rule + carry yoke pin aeea6168'
seq: 2833
status: done
priority: p0
labels:
    - accounts
created: 2026-10-29T10:49:18.028192Z
weave: 337
assignee: claude-sonnet5.5
sprint: 524
sprint_id: 9c8c2767-6917-5a65-bb86-481e65ce919a
sprint_title: Bashy account inventory, operational selection, and app summary
closed: 2026-10-29T11:43:56.853093Z
closed_by: s524-manager
---

Part of S524.7 (umbrella story 607ba5d10846, Sprint 524), slice C (bashy): fixture fix + yoke pin carry. Base: bashy main (>= ba25ee19).
1. Pin: bump github.com/qiangli/yoke require to aeea6168 (yoke main; includes S524.6 + S524.8 + S525 work) and the matching pin in internal/agentos/src_mount.go (srcModulePins). Keep github.com/qiangli/yoke/pkg/provider at 96e0a61f unless go mod tidy requires otherwise (say so in the commit body). Commit: `sync: bump yoke pin to aeea6168 (S524 accounts + S525)`.
2. With that pin, internal/agentos TestConnectAccount fails at connect_account_test.go:74 'accounts: account already exists': since S524.6 a host-login identity is (endpoint, user), so the fixture's three alice@owl.example accounts (keyed, nocred, badpw) collide. This rejection is INTENDED. Fix the fixture: give each fixture account a distinct user and/or endpoint (e.g. alice@owl, bob@owl, carol@otter) and keep every assertion's intent. Add one assertion that adding a second alias for the same endpoint+user is refused.
Gates: GOWORK=off go test ./internal/agentos/ -count=1 (full package; TestRunSignaledIsNonLossy is a known pre-existing failure on Dragon — report it but it does not block), GOWORK=off go vet ./internal/agentos/, GOWORK=off go build ./..., GOOS=windows GOWORK=off go build ./... . No per-run GOCACHE. Do NOT push. Every commit carries trailers: Sprint: #524, Story: <this bashy story's #N>, Story-ID: <its full 12-hex id>, Agent: <your binding>.
