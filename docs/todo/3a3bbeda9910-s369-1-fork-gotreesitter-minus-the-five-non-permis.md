---
id: 3a3bbeda9910
kind: task
title: S369.1 Fork gotreesitter minus the five non-permissive grammars; pin it in yoke and bashy
seq: 396
status: assigned
priority: p1
labels:
    - licensing
    - yoke
    - bashy
created: 2026-10-02T22:39:29.188252Z
weave: 14
assignee: ycode-glm-5.3
sprint: 369
sprint_id: ac323bec-85c1-53d1-9796-b27703113450
sprint_title: Permissive-only grammar set and Java as a registered fence
---

Goal
yoke/pkg/treesitter links github.com/odvcencio/gotreesitter, which embeds 206 grammar parse tables; yoke/THIRD_PARTY_GRAMMARS.md (generated 2026-10-02 by scripts/grammar-licenses.sh) shows five are non-permissive and are in bashy's shipped bytes today: caddy, disassembly, jq, ebnf (GPL-3.0) and nim (MPL-2.0). Operator decision 2026-10-02: keep the other 201 and attribute; drop these five.

Contract
- gotreesitter has no per-grammar exclusion that removes the bytes (grammar_set_core still embeds caddy, disassembly, ebnf; grammar_subset tags only gate registration; GOTREESITTER_GRAMMAR_SET is a runtime filter). So: a minimal pinned fork (qiangli/gotreesitter) that deletes the five blobs, their *_register.go/*_scanner.go files and lock lines, with a provenance note; replace directive in yoke and bashy go.mod (+ .sibling-pins if the fork is cloned as a sibling). (Attribution regen + fail-closed generator check live in S369.2; the upstream exclusion offer lives in S369.3.)
- Verify: strings on the lean bashy binary finds none of the five blob names; the nine languages yoke registers still parse (go test ./pkg/treesitter); binary size recorded before/after.

Acceptance
- CGO_ENABLED=0 release builds of bashy on all six platforms contain no GPL or MPL grammar bytes (proven by strings on the lean binary); go test ./pkg/treesitter green; binary size recorded before/after. (Attribution-file end state is S369.2's acceptance.)
