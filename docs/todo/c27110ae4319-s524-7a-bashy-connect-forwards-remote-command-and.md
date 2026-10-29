---
id: c27110ae4319
kind: feature
title: 'S524.7a bashy: connect forwards remote command (and user) to outpost ssh'
seq: 2827
status: done
priority: p1
labels:
    - accounts
created: 2026-10-29T08:35:51.794961Z
weave: 331
assignee: muse-conductor
sprint: 524
sprint_id: 9c8c2767-6917-5a65-bb86-481e65ce919a
sprint_title: Bashy account inventory, operational selection, and app summary
closed: 2026-10-29T11:43:52.192605Z
closed_by: s524-manager
---

Part of S524.7 (umbrella story 607ba5d10846, Sprint 524), slice A: `bashy connect` forwards the remote command to outpost ssh. Independent of the accounts store; --account wiring comes in a later slice.

Today bashy/internal/agentos/connect.go refuses any `-- CMD` ("only the interactive shell is currently available") and calls transport("ssh", "--", host). outpost already supports `outpost ssh -- [user@]host[:port] cmd args...` (cobra strips `--`; args[0] is the host, the rest is the remote command, run non-interactively via runRemote; $OUTPOST_SSH_PASSWORD enables non-interactive password auth). user@host already passes through unchanged as the host argument.

Change:
1. When a command follows `--` and --native is NOT set: call transport(ctx, "ssh", "--", host, cmd...) and propagate the child exit status exactly (existing childExitStatus). Keep the arguments as separate argv entries (no local shell joining). Remote args that look like flags (--help, --native, -x) must stay remote.
2. Keep refusing --native (WSL/native selection still needs an outpost extension) with the existing honest message, reworded to mention only --native.
3. Update connectHelp: document `bashy connect [user@]host[:port] -- <cmd> [args...]`, that outpost joins the remote argv with spaces like ssh (quote accordingly), and `$OUTPOST_SSH_PASSWORD` for agents (never put a password on the command line). Examples use placeholder hosts only (owl, otter, dev@owl).
4. Update/extend connect_test.go (and connect_unix_test.go if relevant) red/green: forwarded argv for host, user@host, user@host:port with a command; remote `--help`/`--native` after `--` stay remote; exit status propagation (non-zero remote exit); --native still refused; no command => interactive path unchanged (transport("ssh","--",host)).
Gates: `go test ./internal/agentos/ -run Connect` and `go build ./...` and `go vet ./internal/agentos/` in bashy; GOOS=windows go build ./... . Do not touch any other command. Commit trailers: Sprint: #524, Story: S524.7, Story-ID: 607ba5d10846 (plus this bashy story's id). Do NOT push.
