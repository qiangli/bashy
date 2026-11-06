---
id: dc1e8ca9c503
kind: spike
title: Design an explicit Windows PowerShell 5.1 source-provider lane
seq: 3098
status: assigned
priority: p0
labels:
    - windows
    - powershell
created: 2026-11-05T12:12:55.487363Z
assignee: codex-gpt6-astra
sprint: 573
sprint_id: a5f60348-0dc8-5af7-80aa-9dffe2c745ea
sprint_title: Complete Windows, C# and PowerShell fence validation
---

Read-only provider-support design for source-required Windows PowerShell 5.1 examples within Sprint562 all-fenced goal. This is separate from Story112 preparation; no runtime, provider substitution, installation, host feature or system mutation is authorized. Preserve all400 Windows source obligations and the existing pinned PowerShell7.6.6 lane. Determine exact source-required5.1 IDs and command/version evidence from pinned PowerShell-Docs a3de8f22552170e70852470d46cd52cd9ca471ec, notably ps101/01-getting-started.md lines158 and168-174 explicitly WindowsPowerShell5.1; ps101/02-help-system.md L194 Get-Help Get-EventLog. Current E4c18 native oracle pins7.6.6. Microsoft migration documentation says EventLog cmdlets were removed from PowerShell7. Do not substitute Get-WinEvent or rewrite authoredsource.
Actual prerequisite evidence: help30s timed out with empty streams, so this is NOT observed command absence; four other native prerequisite groups passed. Keep source-version mismatch separate from timeout cause. First read-only source audit; then inspect existing Windows5.1 executable identity/version/availability on the already-authorized Windows test route using only harmless no-profile identity discovery after manager scheduling. No new provider install, command import/compatibility session or example execution. Preserve exact provider hashes, architecture, executable/version and built-in module availability evidence privately, no machine/user/path details in publicdocs.
Inspect current sh PowerShell runtime/provisioning/callback/typed/IPC/cleanup/locale seams against5.1 syntax/.NET compatibility. Propose the smallest ADDITIVE explicit provider-selection lane, pinned immutable existing provider and native oracle pairing, never silently replacing7.6.6. Cost files/API/milestones and tests, boundaries for unsupported features, exactsource/expected/status bindings, and ownership/cleanup for anyfuturecompatibility processes. No implementation before manager review. No native acceptance by worker. Deliver exactcase-ID map, concrete existing-provider discovery command for parent, compatibility gaps and boundeddesign. Manager alone executes Windows commands/cache-affecting gates. Work isolated from all current sharedruntime/corpus branches. Source-only commits with DCO and Sprint562/story trailers; no canonicalpublish/pin changes.
