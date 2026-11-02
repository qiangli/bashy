---
id: 0f8ecab1247b
kind: bug
title: S557 restore full-gate session and path fixtures
seq: 3036
status: wontfix
priority: p1
labels:
    - superseded
created: 2026-11-01T17:20:31.908121Z
assignee: s557-manager
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
closed: 2026-11-02T05:32:08.156218Z
---

Full gate session test uses insecure shared /tmp socket directory and drops Serve errors; use unique private short socket directory and retain startup errors without weakening secureSocketDir. Outpost-profile Windows conversion fixture depends on fake PATH cygpath shadowed by real in-process implementation; test valid real conversion inputs or explicit narrow injection while preserving behavior assertions. Full gate bootstrap separately builds exact product and uses standalone module graph; no skips or arbitrary expectation changes.

## Superseded by consolidation — 2026-11-02

Operator requested consolidation of implementation repairs into acceptance owners. Scope, original acceptance, unresolved work and evidence transfer to public umbrella #6, story `bf7624a5c2f2`. This record is closed by decision with label `superseded`; it is NOT a delivery acceptance, test pass, waiver or feature retirement. The parent remains open. Continue corrections under that parent rather than creating another repair story. Original content above is retained unchanged.
