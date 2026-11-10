---
id: 90a7e10abbab
kind: feature
title: 'S583.5 Knowledge and memory: per-circle store isolation and recall-to-prompt boundary (kb, recall, search, graph, chat)'
seq: 3136
status: todo
priority: p3
labels:
    - boundary
    - kb
    - boundary-later
created: 2026-11-06T21:45:55.92881Z
---

Knowledge and memory: prevent cross-circle recall into prompts by physical store partitioning via yoke/pkg/scope.Resolve (~/.bashy/kb/ for default circle, ~/.bashy/kb/owner/ at 0700, ~/.bashy/kb/circles/<circle>/ for family/friend/team/guest) so kb/index.md, journal.jsonl, and .git never mix circles on disk. Repo kb (<repo>/docs/kb) inherits the repo's circle (.bashy/circle); kb agent ring keeps its owner-only Source.Tool check and scopes by active circle (yoke/pkg/kb/store.go, search.go). Filter remember/recall, kbrecall/kbmemory in bashy/internal/agentos, graph store, search index, and chat history by active circle before anything reaches an agent prompt. Explicit cross-circle sharing uses `bashy circle share <slug> --to <circle>` (human approve + scrub gate; no query-time tag-share DSL).
Depends on: S583.2, S583.4.
Acceptance: a kb page or memory written in family is never recalled/searched in a team repo (and vice versa), with no shared index.md/journal.jsonl leakage; explicit `circle share` copies only the approved page to the target circle.
