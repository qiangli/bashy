---
id: ae406925eb87
kind: task
title: 'S7 defensive authoring: one canonical way for an action to refuse agentic mode'
seq: 255
status: todo
priority: p2
created: 2026-09-10T10:45:50.708233Z
sprint: 146
---

The author of an action decides whether it supports agentic mode and what happens when it does not. This story gives that decision one spelling so it is not hand-rolled differently in every script.

THE SITUATION. Environment activation means a script can run under
BASHY_AGENTIC=true without its author ever intending it. That is not a defect to
engineer around - an action declaring no agentic support is already untouched,
so nothing is silently made non-deterministic. But a script whose correctness
DEPENDS on deterministic tooling should be able to say so and fail loudly rather
than proceed.

DELIVER a documented convention plus one canonical idiom for the check, so five
hundred scripts do not invent five hundred preambles. Cover the symmetric case
too: an action that requires agentic mode and finds it off - which is S3's loud
failure, reached from the author's side rather than the runtime's.

Document it as ADVICE with its reasoning, not as a mandatory preamble. Most
actions need nothing: they declare no agentic support and are unaffected.

GATE: the idiom works identically in an interpreted script, a compiled command
and a shell function, and its failure message names the activation surface that
was in effect - an author debugging this needs to know whether it was the
environment, a flag or the source.

Sprint: #146
