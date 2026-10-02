---
id: 3a3bbeda9910
kind: task
title: S369.1 Fork gotreesitter minus the five non-permissive grammars; pin it in yoke and bashy
seq: 396
status: todo
priority: p1
labels:
    - licensing
    - yoke
    - bashy
created: 2026-10-02T22:39:29.188252Z
sprint: 369
sprint_id: ac323bec-85c1-53d1-9796-b27703113450
sprint_title: Permissive-only grammar set and Java as a registered fence
---

Goal
yoke/pkg/treesitter links github.com/odvcencio/gotreesitter, which embeds 206 grammar parse tables; yoke/THIRD_PARTY_GRAMMARS.md (generated 2026-10-02 by scripts/grammar-licenses.sh) shows five are non-permissive and are in bashy's shipped bytes today: caddy, disassembly, jq, ebnf (GPL-3.0) and nim (MPL-2.0). Operator decision 2026-10-02: keep the other 201 and attribute; drop these five.

Contract
- gotreesitter has no per-grammar exclusion that removes the bytes (grammar_set_core still embeds caddy, disassembly, ebnf; grammar_subset tags only gate registration; GOTREESITTER_GRAMMAR_SET is a runtime filter). So: a minimal pinned fork (qiangli/gotreesitter) that deletes the five blobs, their *_register.go/*_scanner.go files and lock lines, with a provenance note; replace directive in yoke and bashy go.mod (+ .sibling-pins if the fork is cloned as a sibling). Offer the exclusion upstream as a build tag so the fork can retire.
- Regenerate THIRD_PARTY_GRAMMARS.md against the fork; the EXCLUDED section must be empty; scripts/grammar-licenses.sh exits non-zero if it is not (add that check).
- Verify: strings on the lean bashy binary finds none of the five blob names; the nine languages yoke registers still parse (go test ./pkg/treesitter); binary size recorded before/after.

Acceptance
- CGO_ENABLED=0 release builds of bashy on all six platforms contain no GPL or MPL grammar bytes; THIRD_PARTY_GRAMMARS.md lists 201 permissive grammars and zero excluded; the Sprint 350 SBOM gate reads that file.
