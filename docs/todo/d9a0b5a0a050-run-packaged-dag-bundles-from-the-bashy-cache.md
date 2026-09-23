---
id: d9a0b5a0a050
kind: feature
title: Run packaged DAG bundles from the Bashy cache
seq: 332
status: todo
priority: p1
labels:
    - bashy
    - tools
created: 2026-09-23T22:01:12.108154Z
sprint: 265
sprint_id: bacdca4b-2dbb-5289-bda4-cfb1bc63548b
sprint_title: Agent-mini SWE-bench showcase
---

Add a .bar bundle contract using a top-level dag.md with a main target and no manifest. bashy run <bundle.bar> [args] extracts/caches the archive, reuses an unchanged bundle, safely updates changed contents, then runs the DAG main target with forwarded args. Update agent-mini package to include its dag.md and define main so it can be consumed this way. Include archive validation/path traversal protection and atomic cache publication.
