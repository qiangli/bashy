---
id: 4dfaaef18f4a
kind: feature
title: O2 Join, switch and leave multiple satellites without losing apps
seq: 3059
status: todo
priority: p2
labels:
    - outpost
created: 2026-11-04T06:51:50.122392Z
sprint: 565
sprint_id: 45140e29-11a7-5dac-b634-da49c692c9f6
sprint_title: Satellite-less single-node outpost by default; join and switch between satellites
---

Depends on O1. The owner joins a satellite with an invite, joins more, switches the active one and leaves back to single-node; published apps, their settings and SSO secrets survive every transition; no write to another profile's recovery code (known register --out bug). Related Sprint 563 J8 (zoo to zoo-aws) and F1 (app entry lost from agent.json). Satellite-side gaps are support tickets. KISS/MVP: reuse existing Bashy tool pin, build and service commands; smallest causal change. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
