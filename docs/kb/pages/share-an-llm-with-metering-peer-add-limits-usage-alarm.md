---
id: 01a1c348-83cd-77b4-8ef0-353d674124c0
seq: 20
form: page
type: runbook
title: Share an LLM with metering (peer add, limits, usage, alarm)
description: 'Agent steps to share the model door with a peer and meter it: peer add/share, peer limits, llm usage, usage-alarm schedule and viewer — WHEN sharing LLM capacity with someone else''s machine or watching shared spend (keywords: llm, share, peer, metering, usage, alarm, limits, 429, thresholds)'
tags:
    - llm
    - metering
    - sharing
    - peer
status: candidate
evidence: 'S#602 Story #3204: alarm unit-checked on fixtures via bashy dag examples/llm-usage test; live verbs ship with the yoke 6c414c3d bump'
source:
    tool: muse-spark1.3-j
    episode: ep-122d82e3ddc8b612
    parent_episode: ep-c4f0df9c5348dc75
created: "2026-11-10T02:27:51Z"
---

## For agents

Share the host's model door with a peer's machine, capped and watched:

```sh
bashy llm peer add FIELD_LAPTOP --share allow
bashy llm peer limit FIELD_LAPTOP --tokens-day 2000000 --usd-month 25 --rpm 60
bashy llm usage --since 24h --json
```

Over-limit answers are HTTP 429 with a reason and a reset: the peer backs
off, the owner checks `llm usage` for who burned what.

Metering lives in `examples/llm-usage/`:

- `usage-alarm.bsh` reads `bashy llm usage --json`
  (schema `bashy-llm-usage-v1`: `since`, `principals[]` with `principal`,
  `requests`, `prompt_tokens`, `completion_tokens`, `cached_tokens`,
  `cost_usd`, `unpriced`), compares caps from a small JSON limits file
  (tokens for unpriced principals such as ollama, USD where priced), and
  calls `bashy notify` once on breach. Run it from `bashy schedule` and
  check it against fixtures, never against the live door:
  `bashy dag examples/llm-usage test`.
- `usage-viewer.html` is the shaping hook: one dependency-free page over a
  `bashy-llm-usage-v1` document. Shape the view there, not in the alarm.

## Rules

- A minted peer is NOT shared until `peer share NAME allow`; suspend with
  `deny`, remove with `peer revoke NAME`.
- Provider budgets (e.g. AWS Budgets) are host configuration, not product
  code: keep them in the host setup, out of the example.
- Needs a bashy build with the `llm usage` and `llm peer limit` verbs. On builds without `bashy llm usage` /
  `bashy llm peer limit`, only the alarm's `--usage-file` fixture mode works;
  `bashy dag examples/llm-usage ping` reports what is missing.

Related: [[kb:extend-bashy-small-verbs-bash-skill-scripts-shaping-hooks-and-th]].
