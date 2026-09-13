---
id: 0eb84ac5ca21
kind: task
title: B1 mount kb context|backlinks|doctor|observe|note add|validate --from-gate|transfer; atlas rows; taught surface forbids direct file edits; e2e on scratch stores; .sibling-pins bump
seq: 266
status: todo
priority: p1
created: 2026-09-13T01:31:42.132004Z
sprint: 163
---

Goal: bashy mounts and teaches the new kb surface; nothing ships wired to nothing.

- Mount kb context | backlinks | doctor | observe | note add | validate --from-gate | transfer --from memex (bashy/internal/agentos/agentos.go, kbrecall.go). Atlas rows in coreutils/pkg/atlas (GroupKnowledge, CapJSON) for every new verb; naming_test ratchet stays green (nouns singular).
- Taught surface: skills/bashy, the manifest and the command atlas teach the five stage verbs and the ring/form flags; forbid direct edits of pages/*.md and graph.jsonl by hand from an agent.
- e2e (-tags e2e) against scratch stores only (BASHY_KB_DIR, BASHY_HOME, agent-data env set to temp dirs): kb context budget, doctor flag-never-fix, note add candidate, validate --from-gate refusal.
- .sibling-pins bump to the coreutils commit that carries C1-C8 (pre-push hook refuses drift).

Gate: bashy make test (both extra lanes) ; bashy commands --atlas lists the verbs; CGO_ENABLED=0 GOOS=windows cross-build; installed-binary smoke on the dev host (make install, never hand-copy).
Traps: shared repo — ack with the live lanes before the first bashy edit; re-gate if coreutils moves between gate and install.
Depends on: coreutils C1-C6 pushed and pinned.
