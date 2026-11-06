---
id: a437de22974a
kind: bug
title: S562 E4 PowerShell retain output and status across errors and exit
seq: 3078
status: assigned
priority: p1
created: 2026-11-05T02:48:27.053585Z
assignee: s562-pwsh
sprint: 573
sprint_id: a5f60348-0dc8-5af7-80aa-9dffe2c745ea
sprint_title: Complete Windows, C# and PowerShell fence validation
---

Split implementation lane under E4 df328510d106: fix output before terminating error and exit status handling with safe worker lifecycle. Preserve native oracle semantics; focused regression plus original corpus rows on Linux and Windows. No skips or denominator reductions. Owning module sh PowerShell worker; coordinate separate C/C# workers. Parent E4 remains open until combined corpus gates pass.
