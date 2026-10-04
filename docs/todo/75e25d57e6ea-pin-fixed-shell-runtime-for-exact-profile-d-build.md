---
id: 75e25d57e6ea
kind: bug
title: Pin fixed shell runtime for exact Profile D build
seq: 401
status: done
priority: p0
created: 2026-10-03T01:13:11.810296Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-04T07:13:48.766443Z
closed_by: codex-gpt6-sol
---

The exact approved host repair rejected the staged candidate before TCC: Bashy .sibling-pins still recorded sh=693f29b, so standalone build.sh cloned that old runtime despite umbrella sh=eb39f9d. Bump .sibling-pins to reviewed sh eb39f9d, push Bashy main, pin umbrella dev, merge exact approval, validate transfer and remote phase gate. One-file static CGO build must report the same sh revision and preserve POSIX on.

## Sprint 355 acceptance evidence 2026-10-04

Fixed ENOEXEC shell pin is merged at e705c69; full6 approval used exact sh 90432168 and passed the phase gate. Historical raw journals and any pending formal certification decisions are unchanged.
