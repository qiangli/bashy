---
id: f3ae837ba6dd
kind: bug
title: Preserve Windows C# worker launch within the native command-line limit
seq: 3094
status: assigned
priority: p0
created: 2026-11-05T10:21:24.894502Z
assignee: codex-gpt6-astra
sprint: 573
sprint_id: a5f60348-0dc8-5af7-80aa-9dffe2c745ea
sprint_title: Complete Windows, C# and PowerShell fence validation
---

The final native Windows C# corpus cannot start its minimal Ready provisioning fence: the pinned PowerShell worker fails CreateProcess with "The filename or extension is too long" before any of the 510 modes run. Diagnose the exact executable/argv UTF-16 length, generated worker script and source identity against the Windows platform limit, preserving the failing native evidence and a small native launch control.

Implement a bounded owning-runtime launch repair only after the measured cause is established. Preserve IPC stdin, stdout/stderr capture, cancellation, cleanup, locale and provider fingerprinting, interpreted/lowered parity and source/module asset behavior. Do not reduce the corpus, shrink required prelude functionality, raise deadlines or substitute an unpinned provider. If a script-file launch is required, specify authenticated private lifecycle/cleanup and exact command argument semantics first. Manager alone runs native Windows and Linux acceptance; workers prepare red/green controls and source. Public artifacts use role names, no private host paths. Proper Sprint/Story/DCO trailers.
