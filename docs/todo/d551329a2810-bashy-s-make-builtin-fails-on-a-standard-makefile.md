---
id: d551329a2810
kind: bug
title: bashy's make builtin fails on a standard Makefile with 'invalid inference rule'
seq: 3338
status: todo
priority: p2
labels:
    - make
created: 2026-11-28T10:23:29.293672Z
---

Seen 2026-11-28 in ClassGo (classgo-public Makefile) during sprint 741 lifts: under bashy, 'make test' runs bashy's builtin make and fails with 'invalid inference rule'; /usr/bin/make test works. Reproduce with the classgo-public Makefile; find which rule the builtin parser misreads (likely a pattern or suffix rule) and match GNU make.
