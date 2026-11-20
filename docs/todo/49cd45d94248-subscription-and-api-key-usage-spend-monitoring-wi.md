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

## Addition (operator, 2026-11-20): auto-prompt the agent

An opt-in flag on the monitor/alert command, for example `--auto-prompt` (with
`--auto-prompt-message FILE` to override the text). When an account an active
worker is using crosses a low-quota or high-spend watermark, bashy also steers
that worker's live session directly. The default message: "Your <account>
quota is low (<level>, forecast <time>): commit work-in-progress on your
branch now and post a STATUS/handoff note to your conductor before
continuing."

Rules:
- Off by default.
- Sent at most once per account per crossing (deduplicated across ticks).
- Uses the steer receipt to confirm the line was submitted. If it was not,
  the alert says so.
- Never sends secrets or the amounts of other accounts.
- Recorded on the room timeline and in the alert itself, so the manager sees
  that the agent was told.
- Works with 26bbbf01: if the agent is already rate-limited, the
  handoff-on-exhaustion path applies instead of a prompt.

Tests: the prompt is sent once per crossing, skipped when the flag is off,
and the receipt is surfaced.
