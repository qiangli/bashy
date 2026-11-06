---
id: 90a7e10abbab
kind: feature
title: 'S583.5 Knowledge and memory: circle labels and recall-to-prompt filter (kb, recall, search, graph, chat)'
seq: 3136
status: todo
priority: p1
labels:
    - boundary
    - kb
created: 2026-11-06T21:45:55.92881Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Knowledge and memory: the largest leak path is recall into a prompt. Label every write (kb all rings, remember/recall, kbrecall/kbmemory in bashy/internal/agentos, graph store, search index, chat history, capability/stats results). Filter by active circle in load/search/recall before anything reaches an agent prompt; kb agent ring keeps its owner-only Source.Tool check and adds the circle check (yoke/pkg/kb/store.go, search.go scopeMatches). Share rules from circles.yaml (e.g. home -> friends: kb tags=recipes) are the only cross-circle path.
Depends on: S583.2.
Acceptance: a kb page or memory written in home is not recalled/searched in an org:acme repo and vice versa; shared-by-rule page is visible only to its target circle.
