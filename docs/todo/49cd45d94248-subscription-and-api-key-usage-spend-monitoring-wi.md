---
id: 49cd45d94248
kind: feature
title: Subscription and API-key usage/spend monitoring with low-watermark alerts to the manager and operator
seq: 3276
status: todo
priority: p2
created: 2026-11-20T23:38:48.570104Z
---

Operator request 2026-11-20. Build on what exists - no new store or transport: yoke pkg/llmbudget (SubscriptionHeadroom = measured remaining fraction of a vendor subscription, forecast, analyzer), 'bashy sprint monitor' (shared accounts and competing work; observe-only today, e.g. '12 accounts, 0 alerts, 15 unknown'), 'bashy llm usage' (gateway requests/tokens/cost by principal), 'bashy llm peer limit' (hard caps), and fleet usage_signals (vendor limit messages).
Gaps: (1) no configured low watermarks - per account: subscription headroom below N% or forecast exhaustion before the next reset; API keys: spend above USD X per day/month or above N% of a budget; (2) no notification - crossing a watermark should post one deduplicated alert (mb to the conductor seat + operator, and a room status event) with the account, current level, forecast and suggested action (shift work to a flat-billed agent, pause metered work, wait for reset); (3) '15 unknown' accounts - report which accounts have no measurement source and why. Never print or log key material; amounts and percentages only. Tests: fixtures for headroom crossings, spend crossings, dedupe across ticks, unknown-source reporting. Related: 6254c3c6 (live rate-limited state), 26bbbf01 (handoff on exhaustion).
