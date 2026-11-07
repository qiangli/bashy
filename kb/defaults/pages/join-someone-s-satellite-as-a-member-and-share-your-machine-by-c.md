---
id: 01a1b833-db45-77cc-a1d4-59cce97e30c2
seq: 17
form: page
type: runbook
title: Join someone's satellite as a member and share your machine by choice
description: 'Member and owner steps for a class or team on one satellite: install, pair with your OWN account, mutual membership, sign in to the satellite, share only what you choose — WHEN joining a class/team satellite or onboarding members (keywords: student, class, member, join, satellite, pair, own account, invite, membership, share, privacy, teacher, owner)'
tags:
    - satellite
    - outpost
    - onboarding
    - class
status: candidate
evidence: 'S587 C2 2026-11-07: CLI flags checked with --help; delegated member sign-in and share-then-access verified live on a satellite with a second account; own-account ownership and mutual membership from code and operator rules; full run pending the S587 dry run (C6)'
source:
    tool: claude
created: "2026-11-07T22:49:28Z"
---

## Who this is for

Anyone joining someone else's satellite (a class, a team) with their own machine. The satellite owner (teacher, team lead) runs the satellite; each member keeps full control of their own machine and shares only what they choose.

Variables used below: `$SATELLITE_URL` (the satellite's address, from the owner), `$OWNER_EMAIL` (the owner's cloudbox account), `$PORTAL_URL` (your own cloudbox portal, default https://ai.dhnt.io).

## Member (student) steps

1. Install Bashy (bashy, outpost, bash, sh). A first install also sets up the default commands and kb pages.

```sh
curl -fsSL https://dhnt.io/install.sh | sh          # macOS / Linux
irm https://dhnt.io/install.ps1 | iex                # Windows PowerShell
```

2. Pair the machine with YOUR OWN account, never with a code from the satellite owner. Sign in to `$PORTAL_URL`, generate an invite code on your Hosts page, then:

```sh
bashy login --code "$CODE"        # same as: outpost register --code "$CODE"
outpost status                    # paired, online
```

A machine paired through someone else's code belongs to that account. Pairing with your own keeps it yours.

3. Become members of each other. Membership is mutual: the owner adds your email and you add theirs (accepting the owner's invitation does both).

4. Open `$SATELLITE_URL` and choose Sign in. You sign in with your own account (delegated through your portal). Apps the owner shared with you appear as tiles; an app's address is `$SATELLITE_URL/matrix/h/<host>/app/<app>/`.

5. Nothing on your machine is shared by default beyond what identifies it for troubleshooting (host name, alias, online status, the names of apps you share). To share an app you run (for example a group whiteboard), publish it and then share it with chosen members from your Hosts page:

```sh
outpost apps add whiteboard --url http://127.0.0.1:18093 --require-login --trust-cloud-identity
outpost apps list
```

6. Optional hardening: turn off built-in services you do not use.

```sh
outpost builtins set --service podman=off --service ollama=off --service ollama-pool=off \
  --service files=off --service meet=off --shell off --desktop off --clipboard off
```

7. When something breaks, run `outpost status` (or `bashy doctor`) and paste the output where the owner can see it (for example the class chat).

## Owner (teacher) steps

1. Add each member's email on the satellite's Members page (they accept, or add you back).
2. Share each class app (chat, boards) with each member from the Hosts page of the host that runs it.
3. Never hand members a pairing code from your satellite or your account: their machines would become yours.
4. Small groups: a group leader can run the group's apps on their own machine and share them with the group's members.

Candidate Bash# command: `satellite-join.bsh` (install check, own-account pairing with a code, status, optional hardening) if the CLI half grows.
