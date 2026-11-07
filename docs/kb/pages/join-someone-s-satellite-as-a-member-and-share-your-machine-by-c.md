---
id: 01a1b833-db45-77cc-a1d4-59cce97e30c2
seq: 17
form: page
type: runbook
title: Join a satellite as a member and share your machine by choice (satellite-member)
description: 'Agent steps for a member joining a satellite another account owns, or an owner bringing members in, via the satellite-member Bash# command — WHEN a user asks to join, onboard to, or share on someone else''s satellite (keywords: satellite, member, owner, join, invite, pair, own account, membership, share, privacy, satellite-member)'
tags:
    - satellite
    - outpost
    - onboarding
    - member
status: candidate
evidence: 'S587 C2 2026-11-07: CLI flags checked with --help; delegated member sign-in and share-then-access verified live on a satellite with a second account; own-account ownership and mutual membership from code and operator rules; full run pending the S587 dry run (C6)'
source:
    tool: claude
created: "2026-11-07T22:49:28Z"
updated: "2026-11-07T22:59:15Z"
---

## For agents

When a user wants to join a satellite that another account owns (as a member), or an owner wants to bring members in, use the `satellite-member` Bash# command. Its skill holds the full steps for both roles:

```sh
bashy satellite-member --skill          # read the steps first
bashy satellite-member check            # installed? paired?
bashy satellite-member pair --code "$CODE"   # code from the USER'S OWN portal, never the owner's
bashy satellite-member publish "$APP" --port "$PORT"
bashy satellite-member harden           # optional: built-ins off
bashy satellite-member diag             # outpost status for the owner
```

It is a default command (registered by a first install, or `bashy-defaults apply`). Without the registration run it from the source pin: `bashy --bashsharp "${BASHY_HOME:-$HOME/.bashy}/src/bashy/examples/satellite-member/satellite-member.bsh" <verb>`.

## Steps that need the user (UI)

- Generate a pairing code on their own portal's Hosts page.
- Membership is mutual: accept the owner's invitation, or add each other on the Members page.
- Sign in to the satellite with their own account.
- Share a published app with chosen members from the Hosts page (owner and member alike).

## Rules

- A machine belongs to the account whose code paired it: members pair with their own account, never with a code from the satellite owner.
- Nothing is shared by default beyond what identifies a machine for troubleshooting (host name, alias, online status, names of shared apps).
- A member can host apps for a sub-group and share them with that sub-group.

Related: [[kb:satellite-sharing-model-one-owner-mutual-membership-pair-with-your-own-account]], [[kb:set-up-outpost-install-pair-publish-apps-and-keep-a-host-private]].
