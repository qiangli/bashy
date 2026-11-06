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

LLM routing. Each circle binds its allowed model endpoints and keys (llm broker yoke/pkg/broker, tessaro, ollama, sphere/peer). Org data never goes to a personal key or provider, personal data never goes to the org gateway; owner circle defaults to local-only endpoints. llm share --via mesh|cloudbox and sphere pooled peers must be circle members; sticky per-principal bindings become per (principal, circle).
Depends on: S583.2, S583.8.
Acceptance: request from org circle to a personal-bound endpoint refused; llm share to a non-member peer refused.
