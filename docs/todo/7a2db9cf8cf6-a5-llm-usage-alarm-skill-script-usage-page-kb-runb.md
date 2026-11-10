---
id: 7a2db9cf8cf6
kind: feature
title: A5 LLM usage alarm skill-script, usage page, kb runbook 'share an LLM with metering'
seq: 3204
status: done
priority: p2
created: 2026-11-10T01:04:30.743325Z
weave: 62
assignee: muse-spark1.3
sprint: 602
sprint_id: 945d80da-6344-5fa1-8afb-08d6cac5d7a0
sprint_title: 'Metered shared LLM access: per-peer usage, cost and limits on the LLM door (provider-agnostic)'
closed: 2026-11-10T02:32:01.231989Z
closed_by: claude-opus5.5-s602
---

examples/llm-usage/: usage-alarm.bsh skill-script (reads bashy llm usage --json, thresholds from a small JSON file: tokens for unpriced models such as ollama, USD where priced; calls bashy notify; run by bashy schedule) and a usage viewer page (one HTML template over llm usage --json) as the shaping hook. kb runbook 'share an LLM with metering' (peer add, limits, usage, alarm). Provider budgets (e.g. AWS Budgets) are host configuration, not product code. Depends on yoke A3/A4. Tests: skill-script unit check on a fixture JSON. Testing (operator rule): unit tests plus at most one minimal integration test in this story; full suites only at sprint close or release.
Manager note: yoke main has the verbs. bashy llm usage [--peer NAME] [--since 7d|24h|0] --json emits {schema:"bashy-llm-usage-v1", since, principals:[{principal, requests, prompt_tokens, completion_tokens, cached_tokens, cost_usd, unpriced}]}. Limits: bashy llm peer limit NAME --tokens-day N --usd-month N --rpm N (over-limit = HTTP 429 with reason + reset). Keep it small: one .bsh skill-script (alarm), one HTML template, one kb page; follow existing examples/ and docs/kb conventions. Test the alarm against a fixture JSON (no live door). Trailers one per line.
