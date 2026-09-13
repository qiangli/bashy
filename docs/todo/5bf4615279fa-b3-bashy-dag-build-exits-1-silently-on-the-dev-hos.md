---
id: 5bf4615279fa
kind: task
title: B3 bashy dag build exits 1 SILENTLY on the dev host (even -n / --json / fmtcheck target) — found by sprint 163 Y1/Y4
seq: 268
status: todo
priority: p1
created: 2026-09-13T03:06:58.788584Z
sprint: 163
---

Observed 2026-09-13 in ycode (DAG.md validates: 'bashy dag --check' ok 22 targets; 'bashy dag --list' works) — 'bashy dag build', 'bashy dag build -n', 'bashy dag fmtcheck' all exit 1 with ZERO bytes on stdout and stderr (captured separately, stdin /dev/null). The installed ~/.local/bin/bashy is the 34 KB 'bashy signal launcher' (Mach-O, 2026-09-12 18:09) exec'ing the real binary. Two workers burned their last 10 minutes chasing this because it was in their gate. Reproduce: cd ycode && bashy dag build -n; echo $?. Gate for this story: the failure prints its reason (a silent exit 1 is the absence-of-evidence failure mode) and the root cause is fixed or recorded.
