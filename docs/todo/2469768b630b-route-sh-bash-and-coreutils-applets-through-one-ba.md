---
id: 2469768b630b
kind: task
title: Route sh, bash, and coreutils applets through one Bashy executable
seq: 389
status: done
priority: p0
created: 2026-10-02T16:39:11.134352Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-04T07:13:47.68924Z
closed_by: codex-gpt6-sol
---

Implement one downloadable bashy executable containing the shell and Go coreutils. Dispatch strict shell aliases and utility argv0 from the same file; verify PATH semantics, alias byte identity, release artifact, and Profile D stage compatibility. Umbrella Story a5f27731bfcd.

## Sprint 355 acceptance evidence 2026-10-04

Single executable alias routing is merged from cb36afc into current main. Full6 used one static ELF SHA-256 484a0430ae3c69eec95f45b1f0d72f053d9e398caaac8a9c28b0ffb39c578bae for shell and Coreutils routes; stage inventory found zero external routes. Historical raw journals and any pending formal certification decisions are unchanged.
