---
id: f154394d1a9b
kind: feature
title: 'S583.11 LLM routing: circle-bound model endpoints, llm share and sphere membership'
seq: 3142
status: todo
priority: p1
labels:
    - boundary
    - llm
created: 2026-11-06T21:45:56.843988Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

LLM routing. Bind allowed model endpoints and keys per circle via yoke/pkg/accounts (`Account.Links.Models`) and yoke/pkg/broker (tessaro, ollama, sphere/peer). Team/work data never routes to a personal key or provider, family/personal data never routes to a team gateway; owner circle is restricted to local-only endpoints (e.g. local ollama). `llm share --via mesh|cloudbox` and sphere pooled peers must belong to the active circle (family/friend/team/guest); sticky per-principal bindings become per `(principal, circle)`.
Depends on: S583.2, S583.8.
Acceptance: request from team circle to a family-bound endpoint refused; llm share to a non-member peer refused.
