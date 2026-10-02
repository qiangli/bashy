---
id: 375033377f88
kind: requirement
title: Define stable certifiable Bashy core and decoupled optional command lifecycle
seq: 390
status: todo
priority: p0
labels:
    - certification
    - architecture
created: 2026-10-02T20:47:12.729171Z
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
---

Bashy must ship one downloadable app with four explicit layers. Base contains the certified POSIX shell/Bash# language foundation and Coreutils path. Core contains stable generic fence parsing, a versioned language/toolchain runner protocol, and minimal CRUD registration (`add`, `show`/`view`, `set`, `rm`, `verify`) so future language/toolchain registrations are data and pinned external payloads rather than Go imports. Builtin contains first-party registered features such as Genie; optional/experimental contains separately versioned and feature-gated registrations. The certified POSIX configuration must not load extension records into POSIX command resolution or change claimed behavior. Optional-only updates must leave the certified core executable digest and Profile D route manifest unchanged. A deliberate new base/core artifact is warranted for a new GNU Bash baseline, Go toolchain/runtime, Bash# language runtime, fence grammar, or runner protocol; each requires conformance impact review under POSIX policy. Specify exact import/code boundaries, artifact and ABI versions, release gates, user upgrade flow, offline/cache behavior, and migration of current hardcoded `polyglot.RegisterLanguage` and `islandToolchains` tables to CRUD records. Acceptance: reviewed design, implementation story breakdown, measured startup/binary impact, a generic external language registration proof without rebuilding Bashy, and an optional-only digest/route invariance gate. CRUN in user language means CRUD command lifecycle, not OCI crun. No full D rerun before the 64-blocker/9-INSPECT gate passes.
