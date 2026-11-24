---
id: 221aa363eabf
kind: bug
title: 'bashy internal/agentos: 4 tests red on main 532a316a (adapterembed drift, common-options baseline, SBOM license for yoke/pkg/provider, pilot catalog cold dispatch)'
seq: 3308
status: todo
priority: p2
created: 2026-11-24T14:17:08.445349Z
---

Pre-existing on bashy main 532a316a, found by Sprint 691 H1 (fresh runs on the dev box, change absent): TestEmbeddedAdaptersMatchSourceTree (embedded adapters drift; go generate ./internal/agentos/adapterembed), TestCommonOptionsVocabularyReport (fetch -A/--user-agent not in the reviewed short-option baseline), TestReleaseSbomCLI (license gate: yoke/pkg/provider unknown), TestPilotCatalogDispatchE2E (slot store empty after a cold dispatch). The whole package also hits the default 10-minute go test limit. Root causes not yet bisected; confirm in the authoritative lane (sealed dag test on the gate host) before fixing. Tracked separately per the no-regression policy.
