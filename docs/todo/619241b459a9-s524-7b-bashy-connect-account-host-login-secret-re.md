---
id: 619241b459a9
kind: feature
title: 'S524.7b bashy: connect --account host-login (secret ref -> OUTPOST_SSH_PASSWORD for the child only)'
seq: 2832
status: done
priority: p1
labels:
    - accounts
created: 2026-10-29T10:17:23.162836Z
weave: 335
assignee: claude-sonnet5.5
sprint: 524
sprint_id: 9c8c2767-6917-5a65-bb86-481e65ce919a
sprint_title: Bashy account inventory, operational selection, and app summary
closed: 2026-10-29T11:44:06.359341Z
closed_by: s524-manager
---

Part of S524.7 (umbrella story 607ba5d10846, Sprint 524), slice B (bashy + yoke-consumed): explicit `--account` for host login, the thin slice that proves the sprint. Builds on: yoke/pkg/accounts (store, Resolve/Select, host-login category, credential refs), S524.2 `bashy account` wiring, and S524.7a (`bashy connect [user@]host -- cmd` forwards to `outpost ssh`, $OUTPOST_SSH_PASSWORD for non-interactive password auth).
Scope:
1. `bashy connect --account <alias|id> [-- cmd...]`: resolve the account (category host-login) through pkg/accounts for operation "connect"; build the target as user@endpoint (endpoint host[:port]); resolve the credential ONLY for this invocation: secret:<name> -> fetch via yoke pkg/secrets client (Config.Resolve + Client.Get) and pass it to the outpost child ONLY via env OUTPOST_SSH_PASSWORD (never argv, never logged, never printed, never in errors); ssh-key:<fp> -> no env, let outpost use keys. The child env must not leak the secret to anything but that one child.
2. Conflicts fail BEFORE any side effect: `--account` plus an explicit host positional that differs from the account endpoint, or a user@ that differs, exits 2 with a structured error naming the conflict (no secret). Missing credential / unresolved account / wrong category -> exit 2 before exec. No `--account` => behaviour identical to today (test it).
3. A transport seam test (no real outpost): fake transport records argv + env; assert argv = ssh -- user@host[:port] cmd..., env carries OUTPOST_SSH_PASSWORD=<sentinel> only for the child, and the sentinel is absent from stdout/stderr/errors (use pkg/accounts/accountstest sentinel). Concurrent invocations with two different accounts never cross env.
4. Help text + `bashy account resolve <id> --for connect` returns safe metadata only.
Base: bashy main 66d5cde5 (account wiring merged; yoke @96e0a61f). Disk on Dragon is tight: no per-run GOCACHE. Gates: GOWORK=off go test ./internal/agentos/ -count=1 (full package), vet, go build ./..., GOOS=windows GOWORK=off go build ./... . Placeholder names (owl, otter, alice). Do NOT push. Trailers: Sprint: #524, Story: S524.7, Story-ID: 607ba5d10846 (plus this bashy story id).
