---
id: f9dc8b3224ad
kind: feature
title: Add versioned language and toolchain CRUD records to Bashy core
seq: 393
status: todo
priority: p0
labels:
    - architecture
    - language
created: 2026-10-02T21:36:59.819327Z
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
---

Extract generic fence parser/runner interfaces into core and add `kind: language` and `kind: toolchain` record schemas under the existing `bashy commands` lifecycle (add, show/view, set, rm, verify). Records pin payload digest per OS/arch, declare effects and protocol version, validate offline, and share collision/lookup rules with command CRUD. Migrate static `polyglot.RegisterLanguage` rows and `islandToolchains` to data or adapters without changing certified shell route or requiring a Bashy rebuild for a new language. Preserve dynamic fence method discovery and no-PATH tool resolution; reject unsupported protocol major versions.
