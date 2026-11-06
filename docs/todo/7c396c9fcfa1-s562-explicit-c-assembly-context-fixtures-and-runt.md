---
id: 7c396c9fcfa1
kind: feature
title: S562 explicit C# assembly context fixtures and runtime assets
seq: 3092
status: done
priority: p0
created: 2026-11-05T04:48:56.9501Z
assignee: s562-csharp-assemblies
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
closed: 2026-11-05T06:41:56.135951Z
closed_by: codex-gpt6-sol
---

Support source-supplied cross-assembly C# examples without flattening or source DLL-load injection. Propose finite authenticated assembly dependency asset graph, independently compile pinned native assemblies and expected diagnostics, preserve visibility and extern-alias/version identity. Initial protected-internal L88 and cs1705 L108. Review design before broad implementation; no arbitrary external paths or missing dependency success. Linux and native Windows validation required.
