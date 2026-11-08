---
id: 5b0a2a2bf36a
kind: task
title: C5 Shared LLM for agentic tools on student outposts
seq: 3173
status: done
priority: p1
labels:
    - class
created: 2026-11-07T21:11:18.65241Z
assignee: claude-opus5.5-s587
sprint: 587
sprint_id: 6ed807a4-dcaf-50c6-9f4c-843d37538a1e
sprint_title: 'Class satellite for Sunday: students pair, share by choice, small groups, shared LLM and Klatch (existing features only)'
closed: 2026-11-08T00:42:45.715755Z
closed_by: claude-opus5.5-s587
---

Use Sprint 586's bashy llm share --via cloudbox --satellite NAME from the host that holds the Bedrock-backed LLM door; each student's agent tool gets a per-peer credential URL (/k/<cred>). Verify a live 200 from a student's agent through the class satellite. Depends on the Bedrock live check (Sprint 586); fallback for Sunday: a local model on the operator's host shared the same way. KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.

## Result (2026-11-07)

Operator: verify with (1) a mock model service and (2) a local tiny ollama model. Sandbox e2e (members scenario, private satellite repo dev 473deaf): the owner publishes an OpenAI-compatible model service as an outpost app with sign-in + identity and shares it with one member; that member's agent calls /v1/chat/completions through the satellite with its own host:proxy bearer. Mock backend PASS (18/18 run); mock + real ollama smollm2:135m PASS (21/21 run): completion 200, unshared member's agent 403, no credential 401. Finding: bashy's own model door (bashy llm serve/share) serves local models only from an engines build (-tags bashy_engines,embed_runner); the lean release build has no local engine, so the Bedrock/door route stays with Sprint 586.
