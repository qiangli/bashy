---
id: 1283eb5b8a06
kind: task
title: O5 Ship outpost-profile up/down/status to every install through the next release's source pin
seq: 3168
status: todo
priority: p1
labels:
    - outpost
    - release
created: 2026-11-07T20:47:40.692621Z
sprint: 599
sprint_id: b38d7a89-cadd-58e2-8748-66b0e9c2f201
sprint_title: 'S585 follow-ups: native Klatch on the Windows test host, rc.30 end-to-end and UAT, local gates under 30 min'
---

Operator decision 2026-11-07 (rod not fish, software shaping): do not embed outpost-profile in the binary. Every installation fetches Bashy's own release-pinned source (bashy src fetch bashy), and the command is registered from that pin (recipe in the script's Register section and the repo kb runbook connect-one-host-to-several-satellites-at-once-outpost-profile-u). The rc.27 pin (9187e1f0) has only add/use/list. Acceptance: the next release candidate is built from bashy main at or after this commit (cut by Sprint 585 R1, 295f6aecae48); on an installed copy of it, bashy src fetch bashy contains cmd_up, the registration recipe works, and outpost-profile status runs; then validate the kb page. Follow-up (not this sprint): move the instance logic into outpost itself if multi-satellite becomes core, with outpost-profile as a thin wrapper. Delivery commits carry Sprint, Story and Story-ID trailers.
