---
id: 16b9bc2b2dfd
kind: feature
title: S513.1a Matrix chat reusable setup skill and happy-path E2E
seq: 2734
status: done
priority: p0
created: 2026-10-27T17:00:03.591659Z
weave: 295
assignee: codex-gpt6-sol
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
closed: 2026-10-27T18:52:08.617485Z
closed_by: s513-manager
---

Implement the code portion of parent bdccbfaf35ea (Sprint513). Matrix chat is an INDEPENDENT tool from brainstorm and whiteboard; all three can run together. General Bashy feature, not classroom-specific; lern students are real UAT later. Build a reusable native Bash# setup/admin skill/custom command for optional signed tuwunel 1.9.4-bashy.2: service declaration/admission, explicit immutable server_name before first start, idempotent user/room provisioning with registration and federation off, status/start/stop and safe backup/restore guidance. Reuse existing service/tool/secret mechanisms and feed-setup patterns; do not invent core command or OpenClaw dependency. Keep secrets out of logs/argv where supported; no classroom roster. Code scope examples/matrix-chat and focused docs/tests; do not edit brainstorm/whiteboard, fleet pins or shared Cloudbox code. Coordinate provision interface with cce04318ac35 integration lane. First send manager your minimal interface (config fields, how identity provision request works), then implement. MUST ADD runnable happy-path E2E: start actual verified service against isolated data; two users login, create/join/invite conversation and exchange text plus image, rerun setup without duplication, stop/restart preserves messages. Test must assert actual protocol results; no mock-only or missing-tool silent PASS. Native test host access supplied by manager; short unit tests okay locally, no containers/heavy builds on dev host and no production changes. No paid API/infra. Report files, commit, tests with exact commands/results and unfinished live criteria. Read local CLAUDE.md. Commit within this repo with injected identity and supplied trailers; do not push/merge other lanes. Bound initial work to one pointed run; send blocker rather than spin.
