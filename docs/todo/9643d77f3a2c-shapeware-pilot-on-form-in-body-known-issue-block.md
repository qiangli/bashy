---
id: 9643d77f3a2c
kind: spike
title: 'Shapeware pilot on form: in-body known-issue block + one TBD spec, fresh-agent run'
seq: 3312
status: todo
priority: p2
labels:
    - shaping
created: 2026-11-25T10:17:49.05743Z
---

Follow-on to umbrella Sprint 712 (story b35b89620f88, plan only). This is its case (iii): a feature delivered without a new binary. Keep it KISS/MVP: prove the rod once, on form only.

1. Check that the Bash# block syntax (`~~~skill as guide`) accepts another block kind. If not, record the gap before changing anything.
2. Add one known-issue block INSIDE form.bsh (operator decision 2026-11-25: in-body, not a separate file), for bug 64ef2626: symptom, workaround (run form.bsh directly), class 'workaround active', expiry. It retires when 64ef2626 is fixed.
3. Add one TBD spec, from the shaped.json genie example 'Group unanswered required fields at the top', with a check under checks/ that fails today.
4. Fresh agent session, no hints: it should get past the bug in one step, and implement the TBD via `bashy shape wish` → verify → approve so that checks/ pass.

Measure: tool calls to get past the bug (baseline 2026-11-25: 5), whether checks pass, wall time. Results go back to Sprint 712. No new verbs and no framework.

## Addendum 2026-11-25: UI face and bashy-ui-v1 (operator agreed)

- UI is the human face of the Bash# coin: script for machines, skill for agents (`~~~skill as guide`), ui for human eyes (later audio/voice for ears). No new marker: the existing source-block forms are the embedding, `~~~ui as NAME !RUNNER … ~~~` inline or `embed ui "./FILE.yaml" as NAME !RUNNER` by reference. JSON/YAML is the default body (the fish); the runner is a registered command (form/page/chart).
- Step 3 writes the TBD spec as a `~~~feature` block (Intent / Structure / Acceptance; acceptance finds elements by label and role), and the form definition as a `~~~ui … !form` fence.
- Name the existing CLI↔serve contract **bashy-ui-v1** in the form README: the document envelope `{schema_version, kind, title, fields|blocks, actions}` (form fields ∪ page blocks vocabulary) plus the existing kit routes POST /api/action, GET /api/state, GET /api/export, GET /api/events. Extend kit/contracttest to the envelope.
- Known gap: a fence calls `RUNNER <verb> <file>`, but form expects `add --def FILE`. Close it together with 64ef2626 (accept a positional file).
- Acceptance tests that drive a browser run on the gate host, never the dev box.
