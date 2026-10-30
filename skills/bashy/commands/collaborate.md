---
name: collaborate
description: 'Coordinate foreign changes, shared resources, freezes, pin bumps, and inbound blocking requests with other agents before acting.'
metadata:
  requires: "has=bashy"
---

# collaborate — identify the owner and agree before acting

Use this protocol whenever work overlaps another agent or an exclusive resource.
An ad-hoc session must be addressable: set `BASHY_AGENT=<name>` or use
`bashy agent track start`. Use that identity consistently for claims and inbox.

## The loop: DETECT -> IDENTIFY -> CONTACT -> COORDINATE -> RECORD

1. **DETECT** the trigger below before changing shared state.
2. **IDENTIFY** the holder, affected seats, checkout, paths, and resource.
3. **CONTACT** the holder with what you saw, your task, and a proposed sequence.
4. **COORDINATE** release, sequencing, merge, or another venue before proceeding.
   A coordination reply does not grant deletion authority over another's work.
5. **RECORD** the agreement, owner, paths/resources, ETA, and outcome in the
   owning sprint story's continuity; include message references and commit SHAs
   where applicable. Release your claims when done and announce completed changes.

## Apply the loop to these six triggers

### 1. Foreign change

Dirty paths, unknown commits, a moved HEAD, staged pins, or an isolation violation
are coordination signals. Identify ownership with `bashy claim who [PATH]`.
Until S541.b provides that command, use `bashy claim list`, `bashy agent --json`,
`bashy sprint who`, and `git log` with `Agent:` and `Story-ID:` trailers.

    bashy mb send <holder> "seen <paths> in <checkout>; yours? I'm doing <X>; propose <sequence/merge>"

Agree on sequencing or merge. NEVER stash, revert, reset, run `git add -A`, or
commit foreign paths. Commit only your own paths or move to a scratch worktree.
Do not overwrite or destroy another owner's branch, worktree, workspace,
uncommitted changes, or disposable state.

### 2. Exclusive resource busy without a claim

A host, `ollama:<host>`, Podman machine, GPU, droplet, or provider account may be
busy even when the claim list is empty. Do not take it. Identify likely owners,
announce the observed use, and contact them directly:

    bashy mb post --topic announce "Who is using <resource>? Seen <activity>; I need <use>, ETA <t>."
    bashy mb send <likely-owner> "Are you using <resource>? Propose <sequence>."

Only when confirmed free, register it with `bashy resource add NAME --kind K`
if unregistered, then take `bashy claim NAME --intent "<what> sprint N, ETA"`.
Silence is not confirmation that it is free.

### 3. You need exclusive use

Identify and register the resource if necessary, then claim before use:

    bashy claim NAME --intent "<what> sprint N, ETA"

Prefer `bashy claim NAME -- cmd` to hold the resource for the command's lifetime.
On refusal, contact the named holder and use
`bashy claim request NAME -m "<need and proposed sequence>"`, `--wait`, or another venue. Keep long-lived
claims refreshed with `bashy claim refresh NAME` and release when done.
Use `--force` only after contact timeout AND operator notice; record the contact
attempts, notice, reason, and outcome in the sprint story.

### 4. Code freeze, build, or release cut

Identify the affected repos, paths, and seats. Coordinate the window, claim the
path set (and `command:` resource if needed), then announce:

    bashy mb post --topic announce "FREEZE <repos> by <seat> for <why>, ETA <t>, contact mb send <seat>"

Respect an existing freeze: contact its holder and sequence your work. When your
cut completes, release the claims and announce `FREEZE LIFTED` with repos and
resulting SHA/artifact; record the outcome.

### 5. Pin bump or commit/push in a shared checkout

Identify other lanes whose runs use that checkout or pin. Send
`bashy mb send <affected-seat> "<checkout/pin>: propose <change and sequence>, ETA <t>"`
to affected seats before the change and agree on sequencing. Prefer a scratch
worktree. Announce the result afterward with its SHA and record it in continuity.
Commit only your own paths; shared-checkout access does not confer ownership.

### 6. Inbound request

Keep `bashy inbox --as <seat> --watch` running while working, or peek with
`bashy inbox --as <seat>` every few minutes. Identify the requester and handle
blockers first: claims, hosts, and checkouts. Reply promptly: release a held claim,
sequence the work, or explain the hold with an ETA. Record the agreement and
follow through; a single inbox read is not ongoing monitoring.

## Contact timeouts

After about 10 minutes without an answer, resend and cc the owning sprint manager
with `bashy mb send <manager> "<request, holder, attempts, proposed sequence>"`.
After about 20 minutes, escalate with
`bashy inbox human send --status blocked "<resource/paths, contacts, attempts, impact>"`.
Keep doing non-conflicting work; never stall silently. A timeout alone does not
authorize taking busy resources or changing foreign work.

## Known limits

Claims are host-local advisory holds. Register remote outposts and Ollama hosts
as resources and coordinate with users on other hosts. Direct HTTP to Ollama
bypasses claims; absence of a claim does not prove the server is idle.
