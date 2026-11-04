---
id: 4a2055d46796
kind: feature
title: L2 Run release, QA gates and promote on the satellite loom
seq: 3065
status: todo
priority: p1
labels:
    - loom
created: 2026-11-04T14:08:56.326942Z
sprint: 566
sprint_id: 360aa48c-f099-5690-aab0-e922da7680e3
sprint_title: 'Dogfood release infrastructure on our satellite loom: build, QA and rollout without GitHub rate limits'
---

Depends on L1. Mirror bashy, outpost and yoke on the satellite loom with GitHub as push mirror; run the release workflow on loom act_runners for every target OS; publish artifacts on the satellite; QA pollers and promote read and write there. Byte-identical promotion rule unchanged. KISS/MVP: reuse S515 loom (satellite loom app, mirrors, peer storage, act_runner) and the existing release.yml/promote.yml/QA-poller contracts; no new CI system. Record exact source, artifact and host identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
