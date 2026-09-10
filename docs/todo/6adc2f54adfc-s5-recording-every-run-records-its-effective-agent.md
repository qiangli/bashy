---
id: 6adc2f54adfc
kind: task
title: 'S5 recording: every run records its effective agentic mode'
seq: 254
status: todo
priority: p1
created: 2026-09-10T10:45:50.682897Z
sprint: 146
---

Load-bearing, not hygiene. Once the environment can activate agentic mode, the SOURCE alone no longer tells you whether a run was assisted - so the RUN has to say so.

The reason it matters: an opted-in action may return model-corrected data
through command substitution straight into a script's dataflow. A wrong result
must be traceable to the run that produced it. Without recording, an assisted
result is indistinguishable after the fact from a deterministic one.

RECORD the effective mode - and which surface set it (environment, flag or
source) - to the three sinks bashy already has: execlog, the audit log, and
OTel. No new sink, no new scope, no new plane.

Where a rung is involved, record the rung that actually resolved, not the rung
the action was permitted to reach. Permitted and used are different facts, and
only the second explains an output.

GATE: run one action under each activation surface and assert the mode and
surface appear in all three sinks; run it with agentic off and assert the same
record shows off. An absent record must not read as off - absence and off are
different, and a success state reached through absence of evidence is the
failure mode this tree has already paid for.

Sprint: #146
