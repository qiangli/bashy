---
id: 65684e7f4df6
kind: refactor
title: Split certifiable Bashy base and core imports from optional initialization
seq: 392
status: done
priority: p0
labels:
    - certification
    - performance
created: 2026-10-02T21:25:36.364454Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-04T07:13:48.228069Z
closed_by: codex-gpt6-sol
---

Implement first bounded patch of architecture story #390: the one-file cmd/bashy certified sh/applet routes must exclude AgentOS, ycode, Genie, and optional feature package initialization from their import graph. Preserve POSIX shell and Coreutils semantics, inherited signals, and minimal command CRUD/front-door where required. Build a core-only one-file profile with focused route tests, measured startup/binary reduction on macOS and Linux, and a documented path to optional records. Do not migrate the generic language protocol or run a full Profile D suite in this story.

## Sprint 355 acceptance evidence 2026-10-04

First bounded base/core import boundary is merged at 2146b93 and c197349 with route/import tests, and measured startup/binary impact is documented. The guarded base profile passed full6. Generic language protocol remains separate story f9dc8b3224ad. Historical raw journals and any pending formal certification decisions are unchanged.
