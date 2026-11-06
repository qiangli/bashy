---
id: 75fa2f23894b
kind: feature
title: K1 Ship Klatch to the fleet in a Bashy release
seq: 3124
status: todo
priority: p1
labels:
    - klatch
created: 2026-11-06T18:13:56.305486Z
sprint: 577
sprint_id: d48e26ca-6b5d-50ba-a074-6af09d3fd177
sprint_title: 'Klatch follow-up: fleet release, outpost and satellite support, product limitations'
---

Pin outpost e4d3e6a (F1: 12 h credential rotation erased apps added since boot; proven on rc.24-dev canary 11-04) in bashy main next to the Klatch v1.2.6 build pin (8c3cd6d0) and yoke pin 698d3161 (browser-hub service, hub --live-port fix); reconcile local bashy main with origin together with S562/S576 and push once; cut the next Bashy release; run QA pollers on elephant and puppy (not dragon); promote; move elephant from the shaped klatch:1.2.5-sso-team2 container to the released Klatch. Verify the elephant outpost build that replaced rc.24 on 11-05 (v1.0.0-rc.26-dev, commit a0b4a1e6) keeps the F1 fix. Carried from Sprint 563 (docs/plan-s563-master-execution.md). KISS/MVP: reuse existing code and harnesses; smallest causal repair. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
