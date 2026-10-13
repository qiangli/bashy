---
id: a889068aea7b
kind: bug
title: Admit exact pinned NNG WebSocket-disabled stub in Prima license review
seq: 1916
status: done
priority: p0
created: 2026-10-13T15:58:00Z
weave: 94
assignee: codex-gpt6-sol
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T16:20:21.138036Z
closed_by: s436-codex-manager
---

Hosted candidate 0.1.1-3 run 38424681793: four build/no-egress lanes PASS; after 9m ScanCode audit only four unreviewed findings remain, one per platform: `deps/nng/src/supplemental/websocket/stub.c` reported `mit AND cpal-1.0`. Disabling WS intentionally compiles this NNG fallback stub. Pinned nng v1.9.0 @29aff2 file has explicit MIT header and SHA-256 `ba3d1139e784b4913095d0f9bb161e6d493513d0a6e96e6d8e507659a56099ee`; compiler input artifact matches. Add only this exact file SHA to `builds/prima.cpp/files/license-review.json` and generated fork; extend focused fixture/tests so the four platform stub findings are accepted only at that hash, altered bytes/CPAL text still fail. Preserve all other fail-closed license rules and WS/SHA1 exclusion. Commit worker story/code with trailers, no push, no Dragon native build/container/full suite. Manager merges, gates, immutable -4 hosted run.

## Implementation plan

Verify the pinned NNG source and stub hash. Add the exact source bytes to the
review fixture and the one path/hash pair to both review maps. Exercise
acceptance and mutation rejection on all four platforms, run the bounded Prima
gate, and commit this story with the code for manager integration.

## Delivery evidence

- Local pinned NNG checkout is commit `29aff2bacc9e2cfa6dcc52155d2816582617f918`; `stub.c` SHA-256 is exactly `ba3d1139e784b4913095d0f9bb161e6d493513d0a6e96e6d8e507659a56099ee` and its header explicitly states MIT terms.
- The existing exact path/hash, pinned NNG license, source pin and expression checks accept the new stub on all four platforms. Focused tests reject changed bytes, added CPAL text and a CPAL-only expression for each platform. SHA1 exclusion and WS OFF/TCP ON assertions remain.
- `bashy gate --command 'bash builds/check.sh prima.cpp && python3 -B builds/prima.cpp/check_test.py' --json` returned `verdict: passed`, `checks[0].passed: true`, exit 0. The bounded check covers 11 shared egress cases, 19 toolchain cases, generated fork reproduction and 49 Python tests. No native build, container, full suite or push was run.
- Manager owns merge, gate and immutable 0.1.1-4 hosted run; hosted audit and signing remain pending.
