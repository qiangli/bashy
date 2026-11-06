---
id: 38ff35b7dc6e
kind: feature
title: S562 E4 C++20 named-module assets and corpus coverage
seq: 3081
status: done
priority: p1
created: 2026-11-05T03:28:49.491133Z
assignee: codex-gpt6-sol
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
closed: 2026-11-06T08:19:10.208108Z
closed_by: codex-gpt6-sol
---

Dedicated dependency of E4df328510d106. Reviewed design c13c47f4 plus successful pinned Zig0.16 module probe: explicit finite module assets/BMI graph and ordinary consumer bridge, preserving exact pinned source and existing ABI/worker protocol. Seven declaration/interface/partition build checks and two custom native-oracle consumers; dependent fragment companion source refs reconciled. Same compiler/target/flags across phases, full validation, no timeout growth. Two std/std.compat examples explicitly require C++23 outside declared C++20 contract: reviewed version-requirement dispositions with source proof, never PASS credit. Implement bounded provider adapter (internal cc1 config capture/replay if required by pinned driver), focused positive/negative tests and Linux real parity before corpus regeneration. No canonical/pin changes during S561 freeze.
