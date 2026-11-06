---
id: 759e75a52741
kind: feature
title: S562 E4 C# compilation-unit and unsafe fence support
seq: 3079
status: assigned
priority: p1
created: 2026-11-05T02:49:14.513898Z
assignee: s562-csharp
sprint: 573
sprint_id: a5f60348-0dc8-5af7-80aa-9dffe2c745ea
sprint_title: Complete Windows, C# and PowerShell fence validation
---

Dedicated implementation dependency of E4df328510d106. Design review then bounded support for namespaces/top-level types/static extension methods and unsafe compiler option, preserving fence exports and error behavior. Restore affected source-bound examples, focused tests and full required Linux/Windows corpus gates. No denominator reduction. Owning sh C# worker, coordinate PowerShell shared worker code.
