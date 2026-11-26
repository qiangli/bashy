---
id: 01a20de9-e3bf-74f1-aa25-d7a74673f826
seq: 34
form: page
type: runbook
title: 'Handle model rate-limit and quota errors (429): switch model, then vendor, keep going'
description: 'What an agent does when a worker or its own model call returns 429, quota reached, rate limit or RESOURCE_EXHAUSTED: confirm, list this host''s bindings dynamically, try another model of the same vendor, then another vendor, relaunch, record one line — WHEN a weave run, sprint assign or fleet probe fails on quota or rate limits (keywords: 429, quota, rate limit, RESOURCE_EXHAUSTED, resets in, failover, fleet, binding, vendor, model)'
tags:
    - fleet
    - quota
    - rate-limit
    - weave
    - routing
status: candidate
evidence: '2026-11-24: a sprint worker hit a per-model 429 (''Individual quota reached, resets in 92h'') one turn in; another model binding of the same vendor CLI probed usable and took the story. Operator guideline: providers meter per model; switch model, then vendor; escalate only when all bindings are exhausted.'
source:
    tool: claude
created: "2026-11-24T14:16:01Z"
---

A 429, "quota reached", "rate limit" or "RESOURCE_EXHAUSTED" from a worker's model is a routing signal, not a failure to report. Providers meter by **model** as well as by account, so another model from the same vendor usually still works. Switch models, then vendors, and keep the work moving. Escalate to the human only when every usable binding on this host is exhausted.

## Steps

1. **Confirm it is a quota error and not something else.** Read the worker's last output: HTTP 429, `quota`, `rate limit`, `RESOURCE_EXHAUSTED`, or a "resets in ..." hint. An auth prompt, a stall with no output or a crash is a different problem (see `bashy weave fleet --auth`). Note the reset time if one is given.
2. **List this host's bindings.** Never assume a vendor list; each user subscribes to different vendors.

   ```bash
   bashy agent list                      # NAME, band, TOOL (vendor CLI), model per binding
   bashy weave fleet --agents            # availability: installed, signed in, cooling down
   ```

3. **Try another model from the same vendor first.** Pick bindings whose TOOL matches the failed one but whose model differs, at the band the work needs. Probe each candidate **one at a time**, because parallel probes and assigns race the probe gate:

   ```bash
   bashy weave fleet --fleet <binding> --probe
   ```

4. **Then switch vendors.** Once every model of that vendor is confirmed exhausted or unusable, choose a binding from another TOOL in `bashy agent list` and probe it the same way.
5. **Relaunch the work on the usable binding.**
   - A run that died with no commits: `bashy weave abandon <run> --force --yes`, then assign the story again to the new binding.
   - A run with commits: resume it, or hand it off with `bashy weave start --resume --run <run> --handoff-to <binding>`.
6. **Record the switch** in one line on the run or sprint thread: the old binding, the reset time and the new binding. This is a note so the next agent skips that binding, not a complaint.

## Rules

- Do not wait out a reset while any other usable binding exists.
- Respect the operator's routing rules for which models a tool may serve. Some hosts, for example, allow a vendor CLI to run only that vendor's own models. Those rules are local policy: read them from the host's instructions, never hardcode them.
- Keep the operator's per-vendor caps, such as one worker per vendor at a time, when you switch.
- Report to the human only when every binding is exhausted, and give the earliest reset time.
