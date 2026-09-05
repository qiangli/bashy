---
id: a0d8470a9288
kind: task
title: 'Human lane M3: app UI defects that hide an agent''s output'
seq: 216
status: todo
priority: p0
created: 2026-09-05T17:08:25.915002Z
sprint: 126
---

IN SCOPE as a context defect, not a cosmetic one: a response the operator cannot read is unusable context in their only remote surface. NOT a redesign and NOT new UI — fix rendering defects on the pages that already exist, with agent responses in the /meet/ Meet and Chat tabs as the named example.

USE THE GATE THAT ALREADY EXISTS. `go test ./pkg/webconsole -tags verifydom` drives a real Chrome over the launcher, Settings, every panel and the /meet/ tabs, asserting no page threw. It exists precisely because the byte-level tests read SERVED BYTES and cannot see a cascade, a DOM, or a script that throws — coreutils/CLAUDE.md records four UI defects shipping in one day through that gap, the worst being a pairing fix that stopped the Settings dialog opening while every byte-level test still passed. Add cases to that suite. Do NOT add a UI framework, a second harness, or a new panel.

OPEN: the specific symptoms are the operator OBSERVATIONS and are not yet recorded here. Each one needs its own reproduction before a fix: which page, which tab, what was expected, what rendered. A red verifydom case per symptom first, then the fix — a UI bug fixed without a failing browser case is indistinguishable from one that was never there.
