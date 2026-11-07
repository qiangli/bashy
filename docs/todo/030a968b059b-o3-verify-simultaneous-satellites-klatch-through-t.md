---
id: 030a968b059b
kind: test
title: 'O3 Verify simultaneous satellites: Klatch through two satellites on macOS, Linux and native Windows'
seq: 3060
status: todo
priority: p0
labels:
    - outpost
created: 2026-11-04T06:51:51.034687Z
sprint: 565
sprint_id: 45140e29-11a7-5dac-b634-da49c692c9f6
sprint_title: Satellite-less single-node outpost by default; join and switch between satellites
---

Depends on O2. Main outpost paired with zoo and a second instance with zoo-aws on the same host. Klatch is reachable through both satellites; a member of each satellite signs in with SSO and both chat in the same team topic; down on the second instance leaves the main one working; after a reboot of the Windows test host both instances and the publication come back. Run on macOS (the dev box), a Linux test host and native Windows (elephant, claim it first). Keep raw logs and the exact Bashy, outpost and Klatch identities. Satellite-side gaps become support tickets. KISS/MVP: reuse existing code and harnesses; smallest causal change. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
