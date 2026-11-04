---
id: fae2464b7d98
kind: test
title: L3 Install and roll out the fleet from the satellite; cut one real release end to end
seq: 3066
status: todo
priority: p1
labels:
    - loom
created: 2026-11-04T14:08:57.37252Z
sprint: 566
sprint_id: 360aa48c-f099-5690-aab0-e922da7680e3
sprint_title: 'Dogfood release infrastructure on our satellite loom: build, QA and rollout without GitHub rate limits'
---

Depends on L2. outpost/binmgr auto-install and fleet upgrade fetch from the satellite, not api.github.com; cut one release (e.g. v1.0.0-rc.25) on loom, canary on one host, promote to the fleet; negative: GitHub unreachable or rate-limited does not block build, install or rollout. KISS/MVP: reuse S515 loom (satellite loom app, mirrors, peer storage, act_runner) and the existing release.yml/promote.yml/QA-poller contracts; no new CI system. Record exact source, artifact and host identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
