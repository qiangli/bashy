---
id: 507f874c1204
kind: enhancement
title: 'F4 usage alarm: apply both caps using priced/unpriced token split; live unpriced check'
seq: 3205
status: assigned
priority: p2
created: 2026-11-10T03:47:56.373807Z
weave: 70
assignee: muse-conductor
sprint: 607
sprint_id: d3d7f687-3414-576c-ad29-b45dfc879257
sprint_title: 'Metering follow-ups: one price table, honest cost and alarms, member go-live guards'
---

After yoke F3 and the bashy go.mod bump to it: examples/llm-usage/usage-alarm.bsh evaluates BOTH caps for every principal, independently (today it is if/elif: tokens only when unpriced, USD only when priced). Token cap against unpriced_tokens (fallback when the field is absent: prompt+completion if unpriced, else 0); USD cap against cost_usd for every principal (0 for a fully unpriced principal, so it never fires spuriously). One ALARM line per breached cap. Stay in bashy's jq subset (no --arg). Fixture: add a mixed principal to fixtures/usage-over.json (unpriced true, unpriced_tokens under the token cap, cost_usd over the USD cap) and assert it in dag.md; existing assertions stay unchanged, so the field-less fixture keeps the fallback covered. Gate: bashy dag examples/llm-usage test (needs jq).

Then prove the unpriced branch live once on a local-model door: isolated BASHY_HOME (short path for the unix socket) and a separate port, the engine, one peer, an ollama model, alarm with a small token cap fires. Venue: a test host that is never the dev box and never elephant before S583 and S606 close (choice recorded on the card; see the plan, dhnt-docs docs/sprint-607-master-execution-plan.md). Agent: muse spark1.3 (code), live check by the sprint manager. Work in a weave workspace, never the shared checkout. Tests: unit only in the story; full suites at sprint close (operator rule). Commit trailers one per line.
