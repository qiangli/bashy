---
id: bc800d2023fe
kind: bug
title: 'inbox: live instances name+N appear to share one read cursor (claude+2 mail is acked/consumed by another claude session); name+N is not a registered sender'
seq: 3285
status: assigned
priority: p1
created: 2026-11-21T10:29:53.513924Z
weave: 127
assignee: agy-gemini4-carbon-w127
sprint: 652
sprint_id: 48a699fb-076f-5442-995a-b12987a45bea
sprint_title: Outpost reliability and host-offline alerting on cloudbox
---

Operator question 2026-11-21: 'agent+seq should not share inbox?'. Evidence from conductor session claude+2 (dev host, after umbrella dcb155d6 'bashy <agent> starts name+N when live'): (1) mb:15014, 15016, 15028, 15030, 15069, 15087 addressed to claude+2 were already [acked] on first read; (2) twice the coordination notice said '1 of 2 unread (2 addressed to you)' and an immediate 'bashy inbox --as claude+2' said 'nothing new'; (3) 'mb send --as claude+2' is refused: names no registered agent or person; other live claude sessions on the host: the backup claude-opus5.5 and an interactive claude. Hypothesis (unconfirmed): read state is keyed on the base agent/principal, not the instance, so name, name+1, name+2 share a cursor; alternative: the session's own coordination-delivery hook acks on injection. Root cause first: reproduce with two live instances of one tool (name, name+1), send one message to each, read as each, and record which instance's read acks which. Expected: each instance has its own cursor and can send as itself; mail to the base name stays distinct from mail to name+N. Related: fa286c7b (attribution), dcb155d6.

## Evidence 2 (2026-11-21, a second `bashy claude` session on the dev host): the launched session cannot post as itself at all

Process chain: Terminal → zsh → `bashy claude` (pid 50305) → claude 2.1.295
(Opus 5.5). Facts, root cause verified at source:
1. The room card for this launch (`~/.bashy/room/members/id-Y2xhdWRl.json`)
   has `id=claude`, `nick=claude`, `tool=claude`, `binding=""` and no model,
   even though the banner shows Opus 5.5.
2. The child environment has no `BASHY_AGENT`, `BASHY_INSTANCE`,
   `BASHY_INSTANCE_UUID` or `BASHY_PRINCIPAL`; only `BASHY_MODEL_SESSION` is
   exported. So `declaredAgentIdentity` (yoke/pkg/bus/actor.go) finds nothing.
3. `mb send --as claude` → "names no registered agent or person": the card id
   is the tool name, not a registered sender.
4. `mb send --as claude-opus5.5` → refused. The card has no recorded model
   (actor.go `unknownModel` branch: "refusing to guess among claude seats"),
   and the hint says the seat is held by another live session (see fd3b74c9).

Cause: `bashy <tool>` knows the identity it launched (a tool, a live label
name/name+N, a steer inbox) but neither records the model/binding on the card
nor exports an identity the child can send as. A session started by bashy
therefore has no usable mb identity. Product defect in the launcher, not the
environment.

Expected: `bashy <tool>` resolves the binding (an explicit agent or model, else
the tool's default model) and records it on the card. It opens or attaches an
instance with its own label and cursor, exports `BASHY_AGENT` +
`BASHY_INSTANCE(_UUID)` to the child, and that label is a registered sender
(`mb send` with no `--as` works). Related: fd3b74c9 (misleading hint),
fa286c7b, dcb155d6.

## Sprint 652 assignment, 2026-11-21

Use an isolated, headless run. Sprint 677 remains the priority. Reproduce using
two concurrently live disposable instances, each with its own label. Send one
uniquely marked message to each, observe and acknowledge independently, then
test that each can send as itself. Keep real mailboxes and existing live
instances outside the fixture. Record the exact observed causal path and a
design note on this story and message the Sprint 652 manager before coding.
After design review, build the smallest fix and regression tests in an isolated
branch with provenance trailers. Report the candidate and evidence for review.


### 2026-11-21 implementation acceptance

Use builds and targeted unit tests for implementation closure. Live multi-instance regression, full suites, multi-OS dispatch, and judge or LLM grading are deferred to the epic or milestone integration gate. The existing two-instance reproduction is evidence recorded before this policy change.

Write and run fast unit tests during implementation. The operator calls the
integration gate only when an epic or one of its milestones is complete.

### Manager disposition for run #127

The wrapper killed the headless worker at its 20-minute runtime cap
(exit -1), after the worker committed the isolated reproduction and
design. The three commits counted against the stale local main were two
published gate-policy commits and one documentation commit. Against
current origin/main, the run has one unique commit and no product code.

The reproduction found that named children can inherit the base
principal, turn injection acknowledges withheld messages as well as
shown messages, and the bare tool shortcut launches a child without
sender identity. Explicitly addressed mailboxes had separate cursors.
The documentation commit rebased cleanly onto the updated gate policy
and is preserved as a local salvage reference. Retain the evidence and
revise the design before starting implementation from the merged S677
pin baseline. Preserve the vendor's default model for a bare tool
shortcut.

## Operator decision 2026-11-21: office phone model (main line + extensions)

Operator (typed): "make it work like office phone and extension" ... "go with (a) hunt group and fix it now. just want to model office or company main line model."

- `NAME` = main line (the agent/role as a whole); `NAME+N` = extension (one live instance's direct line).
- Extension: own mailbox + own read cursor; reading on one extension never consumes another's or the main line's mail.
- Caller ID: an instance always sends as its extension (`from NAME+N`); replies go to that extension. `--as NAME+N` works for inbox read and mb send (live session claim still enforced).
- Extensions are keyed by instance UUID internally; `+N` is a display label. A new session reusing `+N` starts with an empty mailbox (never receives a previous session's mail).
- Voicemail, never drop: mail to an extension whose session ended is forwarded to (held on) the main line.
- Main line = HUNT GROUP: a message to `NAME` is claimed by the first live instance that picks it up (exactly once; others see it as taken, with who took it); with no live instance it waits as voicemail until one picks up.
- `NAME+*` = ring-all (explicit broadcast: every live instance gets its own copy).
- Named seats (e.g. a conductor seat) keep today's behaviour.
- Delivery vs acknowledgement tracked separately (from the root-cause note): items withheld from an inline turn stay unread.

Acceptance (unit/hermetic tests, no integration runs): two live instances NAME and NAME+2 plus NAME+3: direct mail to each extension is seen only there; a main-line message is claimed exactly once under concurrent readers; a main-line message with no live instance waits and is claimed when one joins; mail to an ended extension appears on the main line; a new session reusing +2 sees none of the old +2 mail; NAME+* reaches all live instances; mb send as self and --as NAME+N both attribute correctly; the '1 of 2 unread' withheld item stays retrievable.

## Operator addition 2026-11-21: team-lead extensions for unregistered subagents (in scope now)

Operator (typed): "we do not want to register each and every agent ... directly addressing the agent managed by the conductor (so subagents no need to register but reachable like all its subagents work in the same office and reachable by the team leader's number)".

- A team lead (conductor, sprint manager, named seat) is registered once; that is the department number.
- Subagents it starts (weave runs, headless workers, ephemeral clones) are NOT registered; each gets an extension under the lead, derived at launch from the parent lead + the run (e.g. `LEAD+run125`), living only as long as the run.
- Caller ID for a subagent's posts is `LEAD/extension` (never the operator's name or 'anonymous'); it acts within the lead's authorization.
- Mail to the lead's number is the lead's hunt group: the lead picks up or transfers to a member extension.
- When a run ends, its extension's undelivered mail forwards to the lead's line; a later run reusing a number starts empty.

Extra acceptance: a weave run started by a lead posts as `LEAD/<ext>` and receives mail sent to that extension; after the run ends that mail appears on the lead's line; no subagent registration step is required.

Out of scope (filed separately as a todo, not planned): an owner-level company main number answered by any owner-authorized agent.
