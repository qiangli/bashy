---
id: 455135314fb4
kind: bug
title: S557 bound native CI stages independently of cumulative compile time
seq: 3039
status: wontfix
priority: p1
labels:
    - superseded
created: 2026-11-01T17:42:25.600095Z
assignee: s557-public-tooling
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
closed: 2026-11-02T05:32:08.549505Z
---

Official Windows41329133475 completed SourceRoute successfully6.302s then hit20minute aggregate step deadline, after all preceding assertions passed. Preserve identical build, vet and tests; split sequential work into named bounded steps with existing package test timeouts and explicit measured compile allowance. Later a16 Windows passed aggregate10m28, so no SourceRoute product defect established. Validate workflow syntax and command equivalence locally; hosted official matrix verifies actual execution. Do not remove tests or claim prior timeout passed.

Implementation: split the existing aggregate into build/vet (20m), core tests
(20m), Windows fixture containment (10m), and source routing (10m). The original
command order, platform selection, local-bin PATH, and Go test timeouts are
unchanged. Windows run 41329133475 measured 9m04s before tests and 9m49s wall
for agentos, including unexplained post-PASS overhead; SourceRoute itself passed
in 6.302s before the aggregate timeout. No historical timeout was reclassified.

Validation: actionlint and git diff checks passed. Mocked execution of old and
new blocks on Windows/Linux/macOS produced identical command, argument, order,
and PATH traces (7/5/5 commands). No product builds or tests ran locally.
Hosted matrix verification remains for the manager; story stays assigned.

## Superseded by consolidation — 2026-11-02

Operator requested consolidation of implementation repairs into acceptance owners. Scope, original acceptance, unresolved work and evidence transfer to public umbrella #6, story `bf7624a5c2f2`. This record is closed by decision with label `superseded`; it is NOT a delivery acceptance, test pass, waiver or feature retirement. The parent remains open. Continue corrections under that parent rather than creating another repair story. Original content above is retained unchanged.
