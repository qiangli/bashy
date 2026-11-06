---
id: 093a8fc1a2f3
kind: bug
title: S562 B3 PowerShell invocation context and stdin isolation
seq: 3083
status: assigned
priority: p0
created: 2026-11-05T03:50:40.256604Z
assignee: codex-gpt5.6-sol
sprint: 573
sprint_id: a5f60348-0dc8-5af7-80aa-9dffe2c745ea
sprint_title: Complete Windows, C# and PowerShell fence validation
---

Real PS940 baseline920PASS: about/about-psitem-L56 inherits PSItem=command in ForEach-Object Begin/End instead of empty; process outputs otherwise exact. about/about-preference-variables-L913 reads Console.OpenStandardInput, expected empty input yields Count0 validation error rc1, actual20s hang reading workerprotocol. Reproduce on PSd50 first, fix owningworker isolation without weakening native stdin/output/exit/cancel contract. Remote focused redgreen and existing lifecycle/regression coverage, Linux+Windows. Do not rewrite corpus/oracles.
