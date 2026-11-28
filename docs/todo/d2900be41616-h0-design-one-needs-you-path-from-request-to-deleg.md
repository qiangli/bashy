---
id: d2900be41616
kind: doc
title: 'H0 Design: one ''needs you'' path from request to delegate to human channel, reusing what bashy has'
seq: 3340
status: todo
priority: p0
labels:
    - hitl
created: 2026-11-28T14:03:06.670151Z
sprint: 745
sprint_id: 79f14d8a-2015-5a93-ba77-60cb7f70e108
sprint_title: 'Human-in-the-loop routing: a designated delegate answers ''needs you'' and reaches the human on their chosen channels'
---

Inventory and reuse, don't duplicate: the steward role (host-wide accountable seat, the human's continuous contact), bashy inbox human send/list/ack (principal mailbox), bashy notify (bus, subject-only), bashy ask (secure keyboard values), sprint directive (standing operator rules and pre-approvals), the attention view in yoke inbox_attention.go, the Klatch participant command, and the console/board UI. Decide: the request record and its states (open, delegated, answered-by-delegate, escalated, answered-by-human, expired, withdrawn); where it lives (MB/bus vs a store per principal); how a delegate is designated and scoped; the default non-delegable list; the channel adapter contract; how a reply is authenticated per channel; how this maps onto sprint 'Needs you' items and manager turn-end. Output: a short design doc, open questions for the operator. Operator review before any code.
