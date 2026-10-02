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

Bashy must ship one downloadable app whose stable core contains Bash, Bash#, Coreutils, and the minimal `bashy commands` add/show/set/rm/verify mechanism. Optional and experimental features must evolve through the existing pinned command registry or a similarly decoupled payload path, without recompiling or changing the certified core artifact. The certified configuration must prevent extension records from altering POSIX command resolution or claimed behavior. A deliberate core release is warranted for a new GNU Bash baseline, a Go toolchain/runtime change, or a Bash# language/runtime change; every changed core artifact requires conformance impact review under the applicable POSIX policy. Specify the exact code/import boundary, artifact digest/version boundary, release gates, user upgrade flow, and certification change classification. CRUN in this request means the add/view/update/delete command lifecycle, not the OCI container runtime. Acceptance: reviewed design and implementation story breakdown with measured startup and binary impact, plus a check that an optional-only change leaves the core executable digest and Profile D route manifest unchanged. No new full D rerun before the 64-blocker/9-INSPECT gate passes.
