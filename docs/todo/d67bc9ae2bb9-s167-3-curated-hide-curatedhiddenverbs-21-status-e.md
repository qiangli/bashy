---
id: d67bc9ae2bb9
kind: task
title: 'S167.3 curated hide: curatedHiddenVerbs (21, status experimental), shims+dispatch unchanged, peer alias + sphere/podman/docker hidden'
seq: 272
status: done
priority: p1
created: 2026-09-13T20:54:10.48581Z
sprint: 167
closed: 2026-09-13T21:15:56.601171Z
---

internal/agentos/agentos.go curatedHiddenVerbs separate from hiddenFrontDoorVerbs (which strips shims); commandsCatalog/hiddenVerbsCatalog filter; liveAtlas marks hidden:true status:experimental; case sphere,peer; peer in alwaysShimVerbs + verbSynopsis; update commands_test/commands_sections_test/atlas_test pins.
