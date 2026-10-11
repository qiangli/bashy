---
id: 0e8f4f0af66f
kind: bug
title: S16 resolve service secret refs from declared env binding
seq: 1892
status: assigned
priority: p1
labels:
    - bashy
    - service
created: 2026-10-11T09:36:22.19426Z
assignee: codex-gpt6-sol
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
---

Scoped S16 service-runtime fix from exact Linux live gate. A service declaration has secrets: [{env: OPENCLAW_GATEWAY_TOKEN, ref: openclaw/gateway-token}]. resolveUnitRuntime currently calls secrets.ResolveAgentKey(ref), whose model credential conventions do not include the declared env name. A host with OPENCLAW_GATEWAY_TOKEN bound is therefore marked failed: secret OPENCLAW_GATEWAY_TOKEN ref openclaw/gateway-token is not bound. Reuse the generic service secrets schema: first accept a nonempty binding under the declared env name in the monitor environment, otherwise resolve the ref through the existing vault mapping; inject only the declared env key into the child and never log a value. Add focused test for env binding and unbound refusal, then claimed Linux live OpenClaw restart/health/exposure. No service-name branch, no compatibility path. Manager-owned direct fix under Sprint418 PLAN provenance.
