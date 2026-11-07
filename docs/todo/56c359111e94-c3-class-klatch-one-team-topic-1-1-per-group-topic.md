---
id: 56c359111e94
kind: task
title: 'C3 Class Klatch: one team topic, 1:1, per-group topics'
seq: 3171
status: todo
priority: p0
labels:
    - class
created: 2026-11-07T21:11:18.350611Z
sprint: 587
sprint_id: 6ed807a4-dcaf-50c6-9f4c-843d37538a1e
sprint_title: 'Class satellite for Sunday: students pair, share by choice, small groups, shared LLM and Klatch (existing features only)'
---

Publish the existing Klatch (native or container, release with SSO + team mode) on the operator's host through the class satellite, add each student as a member and share the klatch app with them. Team mode puts everyone in the class topic and connects all pairs (1:1). The operator creates one topic per small group with its members. Verify with test accounts: SSO sign-in, class topic, a 1:1 message, a group topic visible only to its members. KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.

Membership is mutual on cloudbox (each party on the other's member list; automatic when an invitation is accepted): the operator invites each student as a member and the student accepts before pairing; shares reach the student only after that. Verify both member lists in the dry run.
