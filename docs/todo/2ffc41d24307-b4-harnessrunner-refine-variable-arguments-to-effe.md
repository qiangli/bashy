---
id: 2ffc41d24307
kind: task
title: 'B4 harnessrunner: refine variable arguments to effect-pure builtins so bashy.run scripts can consume $YCODE_IN_<PORT> (sprint 163 Y1 residue)'
seq: 269
status: todo
priority: p2
created: 2026-09-13T03:06:58.828246Z
---

ycode Y1 (bashy.run) composes typed inputs as 'export YCODE_IN_<PORT>=literal' lines; pkg/harnessrunner's intent compiler treats ANY variable expansion in a command argument as unprovable (staticWord -> dynamicCommand -> Intent.Complete=false) and incomplete preflight is force-denied, so 'printf %s "$YCODE_IN_X"' is denied under the real executor. Fix: a refiner proving that variable arguments to effect-pure builtins (printf/echo/test/…) add no governed effect. Pinned on the ycode side by TestBashyRunEnvExpansionIsIncompleteEvidenceUnderRealBoundary (delete it when this lands). Until then Y2 substitutes typed inputs as quoted literals into with.script.
