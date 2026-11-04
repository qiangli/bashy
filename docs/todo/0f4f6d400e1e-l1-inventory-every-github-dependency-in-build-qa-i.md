---
id: 0f4f6d400e1e
kind: requirement
title: L1 Inventory every GitHub dependency in build, QA, install and rollout
seq: 3064
status: todo
priority: p1
labels:
    - loom
created: 2026-11-04T14:08:55.14547Z
sprint: 566
sprint_id: 360aa48c-f099-5690-aab0-e922da7680e3
sprint_title: 'Dogfood release infrastructure on our satellite loom: build, QA and rollout without GitHub rate limits'
---

List each step that calls GitHub (Actions runners, release asset upload/download, api.github.com/releases/latest in binmgr auto-install, QA refs refs/qa/<version>/<os>, promote, fleet-notify webhook), its rate-limit exposure, and its loom/satellite replacement. Record where loom runs today (S515 satellite loom app) and which hosts can be act_runners (Linux, macOS, native Windows). Evidence: S563 outpost e2e failures from api.github.com HTTP 403 on 11-04. KISS/MVP: reuse S515 loom (satellite loom app, mirrors, peer storage, act_runner) and the existing release.yml/promote.yml/QA-poller contracts; no new CI system. Record exact source, artifact and host identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
