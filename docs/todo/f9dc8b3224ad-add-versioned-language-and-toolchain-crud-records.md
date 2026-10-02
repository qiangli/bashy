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

## Sprint 355 implementation slice

The first vertical slice adds a strict `bashy.extension/v1` YAML record and
`bashy commands language|toolchain add|show|view|set|rm|verify`. It stores
records atomically under the existing fleet root in separate `languages/` and
`toolchains/` namespaces. Add/set validate names and aliases, reserve compiled
fence tags and base toolchains, require declared effects and an exact
per-platform SHA-256, and reject protocol majors other than 1. `verify` reads
only the current platform's local executable and compares its digest; it does
not download or execute. The core probe and full Bashy both expose the CRUD
verbs. The cert profile refuses extension-record access. Focused tests cover
CRUD, collisions, failed updates retaining prior bytes, offline verification,
digest mismatch, and cert exclusion. These records are intentionally inert.

Remaining acceptance: extract the generic fence runner protocol into core;
resolve the pinned payload without PATH and bind one record snapshot to a
unit; prove external language method discovery/invocation and upgrade without
rebuilding Bashy; migrate the static `polyglot.RegisterLanguage` and
`islandToolchains` rows with parity; integrate versioned optional catalogs and
the executable-digest/route invariance gate. Until those are done, this story
stays open and the `bashy_core` profile remains diagnostic-only.
