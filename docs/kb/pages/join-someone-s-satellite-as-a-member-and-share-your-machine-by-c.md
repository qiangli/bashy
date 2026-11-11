---
id: 01a1b833-db45-77cc-a1d4-59cce97e30c2
seq: 17
form: page
type: runbook
title: Join a satellite as a member and share your machine by choice (satellite-member)
description: 'Agent steps for a member joining a satellite another account owns (one command: bashy onboard), or an owner bringing members in, via the satellite-member Bash# command — WHEN a user asks to join, onboard to, or share on someone else''s satellite (keywords: satellite, member, owner, join, onboard, invite, pair, own account, membership, share, privacy, satellite-member)'
tags:
    - satellite
    - outpost
    - onboarding
    - member
status: candidate
evidence: 'S587 C2 2026-11-07: CLI flags checked with --help; delegated member sign-in and share-then-access verified live on a satellite with a second account; own-account ownership and mutual membership from code and operator rules. S606 S4 2026-11-10: reworked to the pair-once + membership + attach flow (yoke docs/cloudbox-members-attach-wire.md); verbs driven against fakes in examples/satellite-member tests, live run pending. S604 C5 2026-11-10: bashy onboard named as the one-command route (examples/onboard/onboard.bsh, go test ./examples/onboard/)'
source:
    tool: claude
created: "2026-11-07T22:49:28Z"
updated: "2026-11-11T00:32:46Z"
---

## One command: `bashy onboard`

For a member bringing their own machine in, `bashy onboard` runs the whole journey below as one resumable sequence (install check, pair once with the user's OWN account, accept the owner's invitation, attach, status). It inspects the state first, skips what is done, and stops at every step only a person can do, printing what to do next; rerun it to continue. Read `bashy onboard --skill` first; `--dry-run` prints the verb calls. Every step is one `satellite-member` verb, detailed below.

## For agents

When a user wants to join a satellite that another account owns (as a member), or an owner wants to bring members in, use the `satellite-member` Bash# command. Its skill holds the full steps for both roles:

```sh
bashy satellite-member --skill               # read the steps first
bashy satellite-member check                 # installed? paired?
bashy satellite-member code                  # mint a pair-once code for the USER'S OWN account
bashy satellite-member pair --code "$CODE"   # pair once with the cloudbox portal (identity only)
bashy satellite-member accept                # list the owner's invitations
bashy satellite-member accept "$INV"         # accept one (membership is mutual)
bashy satellite-member attach "$SAT"         # attach this host to the satellite
bashy satellite-member publish "$APP" --port "$PORT"
bashy satellite-member harden                # optional: built-ins off
bashy satellite-member diag                  # outpost status for the owner
```

It is a default command (registered by a first install, or `bashy-defaults apply`). Without the registration run it from the source pin: `bashy --bashsharp "${BASHY_HOME:-$HOME/.bashy}/src/bashy/examples/satellite-member/satellite-member.bsh" <verb>`.

## The flow (pair once, join by membership, attach)

- The machine pairs ONCE with the cloudbox portal under the member's OWN account (`code` → `pair`): identity record only, no tunnel.
- Membership is mutual: the owner invites (`bashy member invite EMAIL`), the member accepts (`accept <id>`) — both sides are added.
- The satellite is an attachment: `attach <satellite>` (candidates: `bashy outpost satellite list --json`). No satellite pairing code is ever used by a member.
- A legacy satellite code (`pair --server <satellite-url>`) is refused unless `--as-owner-node` is passed explicitly: such a code makes that satellite's owner the owner of this machine.

## Steps that need the user (UI)

- Sign in to bashy with their own account before `code` (or generate the code on their portal's Hosts page).
- Sign in to the satellite with their own account to reach shared apps.
- Share a published app with chosen members from the Hosts page (owner and member alike).

## Rules

- A machine belongs to the account whose code paired it: members pair with their own account, never with a code from the satellite owner.
- Nothing is shared by default beyond what identifies a machine for troubleshooting (host name, alias, online status, names of shared apps).
- A member can host apps for a sub-group and share them with that sub-group.

Related: [[kb:satellite-sharing-model-one-owner-mutual-membership-pair-with-your-own-account]], [[kb:set-up-outpost-install-pair-publish-apps-and-keep-a-host-private]].
