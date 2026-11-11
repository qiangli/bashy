---
id: 8e20e5f907db
kind: bug
title: 'agentos: two pre-existing red tests on bashy main (src pin drift from d00e0faa; tool-fence drift test)'
seq: 3219
status: todo
priority: p1
labels:
    - test
    - agentos
created: 2026-11-11T04:40:03.991137Z
---

Found 2026-11-11 by the S605 conductor while gating weave #80 (examples/tray only; not the cause). Reproduced at base e6078fb2 with GOWORK=off on the macOS dev box, without #80.

1) TestSrcPinsMatchLinkedModules (internal/agentos/src_mount_test.go:12): 'yoke pin is out of sync with the linked module version v0.0.0-20261111002640-a9650b1363a8'. Cause (bisected): d00e0faa (S604, bump yoke to a9650b13) did not update the hand-edited table in internal/agentos/src_mount.go:18 (still f173567d). Passes at d00e0faa~1. Small fix: update the literal; durable fix is todo 8006b239.

2) TestToolFenceRejectsCatalogDriftAfterAnalysis (internal/agentos/polyglot_tool_row_test.go:193): 'drift dispatched: err=<nil> calls=1' — deterministic (3/3). Already red at d00e0faa~1, so older than the yoke bump; not yet bisected. Classify (product vs test defect) before any fix.

Effect: weave pull's suite gate fails for every bashy merge until these are fixed.
