---
id: 0b9dd209cf6b
kind: task
title: C1 Provision the class satellite on AWS under the operator
seq: 3169
status: todo
priority: p0
labels:
    - class
created: 2026-11-07T21:11:18.050244Z
sprint: 587
sprint_id: 6ed807a4-dcaf-50c6-9f4c-843d37538a1e
sprint_title: 'Class satellite for Sunday: students pair, share by choice, small groups, shared LLM and Klatch (existing features only)'
---

Check the master's satellite list first: the master never deletes satellite rows, so a name used before may be stuck (lost) or still point at an old endpoint; pick a fresh name or use the replace flow if so. Then follow the S522 AWS Lightsail runbook (bashy infra provision aws --satellite NAME, dry run first; AWS keys from the vault; budget guard applies). Verify HTTPS, owner sign-in, the satellite listed under the operator, and record the endpoint privately (not in this repo). KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.
