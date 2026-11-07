---
id: fd3b74c91c61
kind: bug
title: 'mb --as refusal: contradictory ''seat held by another live session'' hint when core says nobody holds the seat; sprint-take advice for comms seats'
seq: 3159
status: todo
priority: p2
labels:
    - comms
created: 2026-11-07T03:58:19.474057Z
---

Repro: registered agent with no live claim, under a harness, no BASHY_AGENT: 'bashy mb send --as NAME X msg'. yoke/pkg/bus/actor.go:242 correctly says 'nobody holds that seat — export BASHY_AGENT or run inbox --as NAME'. bashy/internal/agentos/agentos.go:523 then unconditionally appends '; seat NAME is held by another live session...' and its held-live branch (:516) advises 'bashy sprint take'. Effect: agents conclude a sprint/manager seat is required to communicate. It is not: any registered agent joins with 'bashy inbox --as NAME --peek' (same session) or BASHY_AGENT=NAME, verified 2026-11-07 with a temporary non-sprint clone (mb send + receive OK). Fix: append the held-session hint only when a live holder exists; drop sprint take from the comms-seat advice (or say it only applies to a sprint conductor seat). Also: sprint take says 'use this exact name for mb' but does not claim the comms seat; print the inbox --peek step.

Also observed (2026-11-07): a claim taken by `inbox --as NAME --peek` from a harness Bash call is anchored to that call's short-lived parent shell, so it expires before the next tool call and the following `mb send --as NAME` is refused again. `BASHY_AGENT=NAME` per command works reliably. Either anchor harness claims to the session key (CLAUDE_CODE_SESSION_ID) or have the refusal recommend BASHY_AGENT first for harness callers.
