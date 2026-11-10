---
id: 507f874c1204
kind: enhancement
title: 'F4 usage alarm: apply both caps using priced/unpriced token split; live unpriced check'
seq: 3205
status: todo
priority: p2
created: 2026-11-10T03:47:56.373807Z
sprint: 607
sprint_id: d3d7f687-3414-576c-ad29-b45dfc879257
sprint_title: 'Metering follow-ups: one price table, honest cost and alarms, member go-live guards'
---

After yoke F3: examples/llm-usage/usage-alarm.bsh applies the token cap to unpriced_tokens and the USD cap to cost_usd for every principal (fallback to the old fields when absent). Then prove the unpriced branch live once on a local-model door on a test host (never the dev box): isolated BASHY_HOME door with the engine, one peer, an ollama model, alarm with a small token cap fires. Fixture test updated. Agent: muse spark1.3 (code), live check by the sprint manager. Tests: unit only in the story; full suites at sprint close (operator rule). Commit trailers one per line.
