---
id: bc800d2023fe
kind: bug
title: 'inbox: live instances name+N appear to share one read cursor (claude+2 mail is acked/consumed by another claude session); name+N is not a registered sender'
seq: 3285
status: todo
priority: p1
created: 2026-11-21T10:29:53.513924Z
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
