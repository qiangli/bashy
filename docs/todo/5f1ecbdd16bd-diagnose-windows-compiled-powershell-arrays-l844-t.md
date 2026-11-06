---
id: 5f1ecbdd16bd
kind: bug
title: Diagnose Windows compiled PowerShell Arrays L844 timeout
seq: 3097
status: assigned
priority: p0
labels:
    - windows
    - powershell
created: 2026-11-05T11:42:04.907658Z
assignee: codex-gpt5.6-sol
sprint: 573
sprint_id: a5f60348-0dc8-5af7-80aa-9dffe2c745ea
sprint_title: Complete Windows, C# and PowerShell fence validation
---

Diagnose the remaining native Windows compiled PowerShell Arrays L844 timeout without changing source or deadlines. Upstream PowerShell-Docs a3de8f22552170e70852470d46cd52cd9ca471ec, reference/7.6/Microsoft.PowerShell.Core/About/about_Arrays.md#L844. Frozen six-case packet manifest9c4d8ba1e7e0834eeafc5a0ead96d926e8bb45ddb500ef836f16f5512dbbbfa5. Exact runtime f1a41ac0f3dc21a441042bec8bd95cec4a4dbcc9; Windows candidate SHA25611afc4f16e7f159ade88a1bcd99ce4110f104e2ba6426d5e0668c6acee28a616. Repeated compiled20s timeout with empty streams; fresh independent native-before/after tuples agree and service snapshots remain stable. Related L858 run and compiled controls pass. Earlier scratch was cleaned; generated executable must be retained in next diagnostic.
Scope: source-focused read-only triage in an isolated worktree, preserve old worker14 and unrelated fcaad7dc delivery. Inspect lowering and provider startup, propose exact phase instrumentation with owned PID/start, generated source and binary identity, process/stream stages, immutable source/provider/environment bindings. No blanket retry, no deadline growth (product20/native30/build120), no source/golden/output normalization, no host or global cache changes. Manager alone executes remote build/native/product gates under exclusive cache ownership. Prepare concrete manager command and expected receipts; no acceptance execution by worker. Any product repair requires root-cause evidence and bounded design review first. Keep public notes free of private host/user/path details; private evidence coordinates provided by manager separately. Use original registered codex-gpt5.6-sol tool/model, not a model switch. Work only on this new task; do not revisit historical PSItem repair. Sprint562 delivery trailers and DCO required. Completion means identified phase/root cause plus reviewable repair or exact remaining blocker, never matching failed captures.
