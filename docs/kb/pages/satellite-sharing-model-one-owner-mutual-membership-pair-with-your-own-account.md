---
id: 01a1b809-63d4-7860-987d-6115c19a4736
seq: 13
form: page
type: fact
title: 'Satellite sharing model: one owner, mutual membership, pair hosts with your own account'
description: 'Who owns what on a satellite and why users must pair their machines with their own account and join as members — WHEN inviting students or teammates to a satellite, deciding whose pairing code to use, or sharing a host or app (keywords: satellite, owner, member, invitation, share, sharee, pairing code, host owner, students, trust, app url, matrix/h)'
tags:
    - satellite
    - sharing
    - ownership
    - membership
    - outpost
status: candidate
evidence: '2026-11-07: observed live on a cloud satellite (accepted invitation added both sides; a host paired with the owner''s code landed under the owner); sharee elevation per outpost connect --help'
source:
    tool: claude
created: "2026-11-07T22:03:05Z"
---

## The model

- **One owner per satellite.** The owner runs the console (Hosts, invite codes, members).
- **Membership is mutual.** An accepted invitation adds both sides: the member joins the satellite and the satellite appears in the member's account.
- **Pairing decides host ownership.** A host paired with a code minted by the satellite OWNER becomes the satellite owner's host, not the person's who ran `outpost register`.
- So students, team members and other users should: pair their machine with their OWN account (their own code), and join the satellite as a MEMBER. The host stays theirs; they share what they choose.
- Sharing is a trusted relationship: a sharee of a host gets elevation from the share grant without the OS password (`outpost connect` skips the prompt), and a sharee of a `--require-login` app is let through. Share only with people you would hand those rights to.

## Reaching shared apps

- App URL: `$SATELLITE_URL/matrix/h/$HOST_NAME/app/$APP/`. It needs sign-in on that satellite, then elevation for the host.
- The satellite's landing page has no sign-in; owners enter through the master's Satellites page, Open console.
- To serve members of a second satellite without moving the host, run an extra outpost instance (`outpost-profile up`, see that page) rather than re-pairing.
