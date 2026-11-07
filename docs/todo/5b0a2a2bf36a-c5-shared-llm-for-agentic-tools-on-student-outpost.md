---
id: 5b0a2a2bf36a
kind: task
title: C5 Shared LLM for agentic tools on student outposts
seq: 3173
status: todo
priority: p1
labels:
    - class
created: 2026-11-07T21:11:18.65241Z
sprint: 587
sprint_id: 6ed807a4-dcaf-50c6-9f4c-843d37538a1e
sprint_title: 'Class satellite for Sunday: students pair, share by choice, small groups, shared LLM and Klatch (existing features only)'
---

Use Sprint 586's bashy llm share --via cloudbox --satellite NAME from the host that holds the Bedrock-backed LLM door; each student's agent tool gets a per-peer credential URL (/k/<cred>). Verify a live 200 from a student's agent through the class satellite. Depends on the Bedrock live check (Sprint 586); fallback for Sunday: a local model on the operator's host shared the same way. KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.
