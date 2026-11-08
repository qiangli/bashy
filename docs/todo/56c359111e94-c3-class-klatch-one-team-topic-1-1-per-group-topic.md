---
id: 56c359111e94
kind: task
title: 'C3 Class Klatch: one team topic, 1:1, per-group topics'
seq: 3171
status: done
priority: p0
labels:
    - class
created: 2026-11-07T21:11:18.350611Z
assignee: claude-opus5.5-s587
sprint: 587
sprint_id: 6ed807a4-dcaf-50c6-9f4c-843d37538a1e
sprint_title: 'Class satellite for Sunday: students pair, share by choice, small groups, shared LLM and Klatch (existing features only)'
closed: 2026-11-08T01:13:05.437972Z
closed_by: claude-opus5.5-s587
---

Publish the existing Klatch (native or container, release with SSO + team mode) on the operator's host through the class satellite, add each student as a member and share the klatch app with them. Team mode puts everyone in the class topic and connects all pairs (1:1). The operator creates one topic per small group with its members. Verify with test accounts: SSO sign-in, class topic, a 1:1 message, a group topic visible only to its members. KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.

Membership is mutual on cloudbox (each party on the other's member list; automatic when an invitation is accepted): the operator invites each student as a member and the student accepts before pairing; shares reach the student only after that. Verify both member lists in the dry run.

## Progress (2026-11-07)

- Operator decision: serve the class from the Windows test host's existing Klatch (shared with the UAT cohort, same team topic).
- The Windows test host joined the class satellite as an extra outpost instance (outpost-profile add + up --app klatch; admin 17779), next to its zoo-aws instance. The dev box joined as well (instance up, no apps published yet).
- Owner verified in the browser: Klatch SSO through the class satellite returns 200 and lands in the same account and team topic.
- Remaining: invite and share Klatch with students (mutual membership), per-group topics, and a 1:1 check (in C6 with test accounts).
- Side effect found: outpost-profile add restarted each host's main outpost for a few seconds (bug filed on Sprint 565).
- Sandbox e2e (C6): owner + 3 member accounts SSO into the owner's Klatch through the satellite as separate accounts; the team topic holds the members; members are connected one to one. Remaining: a per-group topic visible only to its members, and an actual 1:1 message (Klatch UI).
- Sandbox e2e (private satellite repo dev d625f3e), PASS 26/26 twice: the owner creates a sub-group topic for two members; one posts, the other reads it credited to the poster; a member outside the sub-group is refused (401). A member opens a one-to-one topic and posts; the other member reads it; a third member is refused (401). All through the satellite with fake accounts.
