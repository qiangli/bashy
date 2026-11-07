---
id: 0b9dd209cf6b
kind: task
title: C1 Provision the class satellite on AWS under the operator
seq: 3169
status: done
priority: p0
labels:
    - class
created: 2026-11-07T21:11:18.050244Z
assignee: claude-opus5.5-s587
sprint: 587
sprint_id: 6ed807a4-dcaf-50c6-9f4c-843d37538a1e
sprint_title: 'Class satellite for Sunday: students pair, share by choice, small groups, shared LLM and Klatch (existing features only)'
closed: 2026-11-07T21:47:52.605005Z
closed_by: claude-opus5.5-s587
---

Check the master's satellite list first: the master never deletes satellite rows, so a name used before may be stuck (lost) or still point at an old endpoint; pick a fresh name or use the replace flow if so. Then follow the S522 AWS Lightsail runbook (bashy infra provision aws --satellite NAME, dry run first; AWS keys from the vault; budget guard applies). Verify HTTPS, owner sign-in, the satellite listed under the operator, and record the endpoint privately (not in this repo). KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.

## Progress (2026-11-07)

- Name checked on the master first: not in use by this owner (18 existing satellites, none with this id or an endpoint containing it).
- Registered on the master under the operator (POST /api/v1/satellites, 201).
- Dry run: small_3_0 available in us-west-2 at $12/mo; month-to-date spend $0 within the account cap and the $24 per-instance ceiling.
- Provisioned with bashy infra provision aws (rc.27 bashy), static address reserved first; the satellite answered HTTPS health (mode satellite, v0.18.56) about a minute after boot; the master shows it enabled, key active, endpoint set, recent contact.
- Remaining: owner sign-in through the master console (Open console), then accept. Endpoint recorded privately, not here.
