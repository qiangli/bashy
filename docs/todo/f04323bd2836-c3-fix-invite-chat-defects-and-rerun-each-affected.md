---
id: f04323bd2836
kind: bug
title: C3 Fix invite/chat defects and rerun each affected journey
seq: 3044
status: done
priority: p0
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.953245Z
assignee: claude-opus5.5-s563
sprint: 563
sprint_id: 282b20a5-f15c-55eb-a314-eef9580e19d1
sprint_title: 'Product UAT: clean-install user journeys and defect closure'
closed: 2026-11-06T18:14:43.492272Z
closed_by: claude-opus5.5-s563
---

Depends on C2 findings; continue through final UAT. Priority order: P0 F1 Klatch app entry lost from outpost config, F9 inbox refusal for registered agents (fixed 9adb8acc, merged); P1 single sign-on, team mode and named team owner released in the Klatch fork and pinned by Bashy (F8, no host-scoped shaping left), host-owner setup as a Bashy command replacing the Python provisioner (F4); P2 native Windows Klatch (pure-Go SQLite build F2, Bashy-provided sh/ffmpeg/ImageMagick F3), satellite move J8, shapeable-ready J10; P3 F5-F7, F10-F15 unless they block P0/P1. Cross-link A/B causes to 561/562 and consume their verified fixes; satellite gaps are tickets, never shaping. Acceptance: every in-scope defect fixed with red/green evidence and its affected positive and negative journey rerun. Newly discovered final-UAT failures reopen this work.

Execution plan: docs/plan-s563-master-execution.md (campaign: docs/plan-clean-baseline-campaign.md). KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
