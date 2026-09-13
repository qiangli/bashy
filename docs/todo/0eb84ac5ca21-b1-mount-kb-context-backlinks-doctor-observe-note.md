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

Goal: bashy mounts and teaches the new kb surface; nothing ships wired to nothing. Design of record: dhnt docs/kb-rings-forms-stages.md §7; plan D1/D5.

CORRECTED SCOPE: bashy mounts kb.NewKBCmd() wholesale (internal/agentos/agentos.go ~line 988), so every new pkg/kb verb (C1/C2/C4/C5/C8: --ring/--form flags, backlinks, doctor, note add, observe, validate --from-gate, transfer --from memex) is mounted by the .sibling-pins bump alone — no bashy code. Only the recall-side verb needs a mount.

- Mount `bashy kb context` from pkg/recall via internal/agentos/kbrecall.go exactly as `kb recall` is mounted (suppress kb's scope header; refuse --dir/--repo/--user/--base-dir). Landing 2 injects cmds/graph's CodeRing through recall's Reader-injection point (C6).
- Atlas rows in coreutils/pkg/atlas (GroupKnowledge, CapJSON) for every new verb; naming_test ratchet stays green (nouns singular).
- Taught surface: skills/bashy, the manifest and the command atlas teach the five stage verbs and the ring/form flags; forbid direct edits of pages/*.md and graph.jsonl by hand from an agent.
- e2e (-tags e2e) against scratch stores only (BASHY_KB_DIR, BASHY_HOME, YCODE_DATA_DIR set to temp dirs): kb context budget + golden shape, doctor flag-never-fix, note add candidate, validate --from-gate refusal.
- TWO LANDINGS (plan D5): landing 1 after coreutils C1–C4 are pushed (mount, atlas, taught surface, e2e, .sibling-pins); landing 2 after C5–C8 (CodeRing injection + second pin bump). The pre-push hook refuses .sibling-pins drift.

Gate: bashy make test (both extra lanes); bashy commands --atlas lists the verbs; CGO_ENABLED=0 GOOS=windows cross-build; installed-binary smoke on the dev host (make install, never hand-copy).
Traps: shared repo — ack with the live lanes before the first bashy edit; re-gate if coreutils moves between gate and install.
Depends on: landing 1 — coreutils C1–C4 pushed and pinned; landing 2 — C5–C8.
