---
id: 14d12ff7c860
kind: task
title: S541.a collaborate skill + manager/conductor pointers (bashy)
seq: 2959
status: done
created: 2026-10-30T17:41:36.525358Z
weave: 361
assignee: codex-gpt6-astra
sprint: 541
sprint_id: 3013eae1-4380-53b7-a528-e53732910595
sprint_title: 'Proactive agent collaboration: collaborate skill, worker preamble, claim who'
closed: 2026-10-30T19:07:40.071845Z
closed_by: s541-manager
---

Repo: bashy only (yoke worker preamble + sprintReadyLine moved to S541.c, manager-owned).
Spec: docs/sprint-541-master-execution-plan.md in the dhnt umbrella (protocol table copied below).

DELIVER in bashy:
1. NEW embedded skill bashy/skills/bashy/commands/collaborate.md (+ any registration the other command skills need, e.g. *.dhnt variant / index in skills/bashy/SKILL.md) so `bashy skill show collaborate` renders. Content: loop DETECT -> IDENTIFY -> CONTACT -> COORDINATE -> RECORD, applied to six triggers:
   a) foreign change (dirty paths, unknown commits, moved HEAD, staged pins, isolation violation): identify with `bashy claim who [PATH]` (being added by S541.b; until then claim list / agent --json / sprint who / git log Agent: + Story-ID: trailers); contact `bashy mb send <holder> "seen <paths> in <checkout>; yours? I'm doing <X>; propose <sequence/merge>"`. NEVER stash/revert/reset/add -A/commit foreign paths; commit only your own paths or move to a scratch worktree.
   b) exclusive resource busy without a claim (host, ollama:<host>, podman machine, GPU, droplet, provider account): do not take it; `mb post --topic announce` asking who uses it + mb send likely owner; if confirmed free `bashy resource add` (if unregistered) then `bashy claim NAME --intent`.
   c) you need exclusive use: `bashy claim NAME --intent "<what> sprint N, ETA"`, prefer `claim NAME -- cmd`; on refusal `claim request NAME -m`, `--wait`, or another venue; `--force` only after contact timeout + operator notice, recorded.
   d) code freeze / build / release cut: claim the path set (and command: if needed); `mb post --topic announce "FREEZE <repos> by <seat> for <why>, ETA <t>, contact mb send <seat>"`; announce FREEZE LIFTED when done.
   e) pin bump or commit/push in a shared checkout with another lane's runs: mb send affected seats before; announce after with SHA.
   f) inbound request: `bashy inbox --as <seat>` (watch or peek every few min); blockers first (claims, hosts, checkouts): release, sequence, or explain with ETA.
   Timeouts: no answer ~10m -> resend + cc owning sprint manager; ~20m -> `bashy inbox human send --status blocked`; keep doing non-conflicting work, never stall silently.
   Identity: an ad-hoc session must be addressable (BASHY_AGENT=<name> or `bashy agent track start`).
   Known limits: claims are host-local (register remote outposts/ollama as resources); direct HTTP to ollama bypasses claims.
   Fold the existing "workplace rule" paragraphs (inbox.md:104-106, sprint.md:60-63, conductor.md:132-135) into this skill and replace them with a one-line pointer "follow `bashy skill show collaborate`".
2. One-line pointer in ownerAccountabilityHelp (bashy/internal/agentos/sprint.go ~136-187).
3. Regenerate skillhelp goldens (bashy/internal/agentos/testdata/skillhelp/*) with the repo's existing update mechanism; run the skillhelp tests.
VERIFY: `go test ./internal/agentos -run 'Skill|Sprint.*Help'` (focused) green; `bashy skill show collaborate` renders from the built binary. No other repos.
