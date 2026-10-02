---
id: 1a018e42188d
kind: test
title: Prove external fenced-language runner without rebuilding Bashy
seq: 394
status: todo
priority: p0
labels:
    - architecture
    - language
created: 2026-10-02T21:36:59.913988Z
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
---

Using the generic CRUD/runner protocol from the dependent story, register a representative external language/toolchain on a previously built one-file Bashy, verify its pinned payload, invoke a fence and method-discovery path, then remove it. Assert the executable digest is unchanged before/after registration and the same test works offline from the verified cache; wrong/missing digest and unsupported protocol fail closed. Keep first-party Genie as a builtin registration through the same protocol and verify current behavior.
