---
id: 27d67f85e738
kind: enhancement
title: 'S227.4 Linux image size: measure the scratch baseline, then cut — drop meet SPA embed, treesitter grammar set, CJK charset tables; record per-variant compressed/uncompressed sizes'
seq: 308
status: blocked
priority: p1
labels:
    - linux
    - size
created: 2026-09-20T21:23:20.183985Z
---

**DEFERRED — unlinked from sprint 227 (operator, 2026-09-20: a simple image build of bashy's existing features, KISS).** The bashy_scratch profile is already the lean static build; S227.1 records the measured size per arch. Cuts are a follow-up once a number says they are worth it.

Starting point is unmeasured for the `bashy_scratch` profile: the lean worker is ~121 MB on unix; the Makefile comment puts the floor for cmd/bash near 5 MB (Go runtime ~2.3 MB + interpreter + x/text CJK tables). Everything between is embeds and optional subsystems.

Do, in order, keeping a table (variant · build tags · uncompressed · compressed · `go version -m` SBOM line · what stopped working):
1. baseline: `bashy_scratch` as built by S227.1, `-s -w -trimpath` (already), plus `go tool nm -size` / `goweight`-style attribution of the top 20 packages;
2. `-tags nomeetspa` (or whatever `scripts/build-meet-spa.sh optional` keys on) — the SPA has no place in a headless script runner;
3. treesitter grammars: `grammar_set_core` vs `grammar_blobs_external` per docs/TODO.md — the 9 mapped languages embedded, the long tail via the S227.2 manifest; ast/graph must degrade with a stated limit, never report "no symbols";
4. x/text charset tables: keep what locale-correct globbing needs, gate the rest;
5. anything else the attribution shows (weave? dag examples? embedded docs/help?) — one commit per cut so a regression bisects.
Not on the table: UPX or any packer (breaks `go version -m`, memory-maps badly, flagged by scanners); dropping coreutils.

Acceptance: table in the evidence record; the airgap gate (S227.3) green on the final variant; a decision line per cut (kept / rejected and why); target agreed with the operator from the baseline number — propose ≥30 % as the opening bid, do not claim it before measuring.
