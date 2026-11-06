---
id: 1a6a92fc6208
kind: bug
title: Preserve PowerShell Out-Host output and source-bound diagnostics
seq: 3100
status: assigned
priority: p1
created: 2026-11-06T02:29:34.554724Z
assignee: muse-spark1.3
sprint: 573
sprint_id: a5f60348-0dc8-5af7-80aa-9dffe2c745ea
sprint_title: Complete Windows, C# and PowerShell fence validation
---

Actual source-bound Windows31-case batch produced60/62 product-mode matches with stable native/provider observations and zero timeouts. Sole both-mode failure: PowerShell-Docs reference/docs-conceptual/learn/ps101/03-discovering-objects.md L477. Product loses the Out-Host formatted table and reports a different diagnostic source location. Preserve exact pinned source, stdout/stderr/exit tuple, returned values and original deadlines; diagnose owning worker/capture path and add bounded red/green controls before original run and compiled rerun. Existing approved console-ABI line-ending mapping is the only normalization; do not hide missing output or rewrite source-location errors generically. Full400 obligation inventory remains unchanged; this is a newly measured defect, not an exclusion. Coordinate carried parent83fe3d72eb62 and d73d4ea89087; private actual receipt94a521de6e4a43afa919aebc23f7ba0e, local adjudication retained by manager.
