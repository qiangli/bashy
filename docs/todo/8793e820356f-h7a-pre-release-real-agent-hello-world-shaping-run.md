---
id: 8793e820356f
kind: test
title: H7a Pre-release real-agent Hello World shaping run (one cheapest working agent, budget-capped)
seq: 3246
status: todo
priority: p1
labels:
    - shaping
    - e2e
created: 2026-11-19T16:28:17.460033Z
sprint: 670
sprint_id: 4b474d28-ab40-5a06-8d82-f14e76759a4a
sprint_title: Desktop journey e2e regression gate, then ship v0.18.78 navigation
---

Slice of H7 515557c02505 (which stays in Sprint 579 for the full 24-cell matrix). Before each bashy release, run bashy/examples/shaping/hello-world/e2e-agent.sh once with the cheapest working agent (pick from the probed fleet; record which) over the CLI interface. It goes wish 'Hello, Shaping!' -> real agent edit -> real verify (checks/smoke.bsh) -> preview -> approve -> second principal unchanged -> undo. This catches regressions in the shaping brief, model output parsing and the real BashyDelegate, which CI never exercises. Opt-in (an env flag), capped (one run, and a budget note per release), and evidence recorded in the release packet. A failure blocks the release unless the operator records an override with a reason. Done when: one passing run is recorded against the current release candidate, and the release procedure names the step and its override.
