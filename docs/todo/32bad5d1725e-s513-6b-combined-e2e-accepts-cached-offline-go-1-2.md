---
id: 32bad5d1725e
kind: bug
title: S513.6b combined E2E accepts cached offline Go 1.27 toolchain
seq: 2748
status: done
priority: p0
created: 2026-10-27T18:20:51.585341Z
weave: 308
assignee: codex-gpt6-sol
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
closed: 2026-10-27T18:33:52.351674Z
closed_by: s513-manager
---

Manager grade of bashy#305 3874c449 failed exit 2: tests/collaboration-e2e/run.py forces GOTOOLCHAIN=local and sees Go1.26, although `GOTOOLCHAIN=go1.27.2 GOPROXY=off go version` succeeds from the already cached toolchain. Repair preflight/build to permit a cached Go1.27+ with GOPROXY=off and GOSUMDB=off, while failing closed if no usable offline toolchain exists. Preserve original #305 author by fetching exact commit/branch from issue-305 into isolated child; do not edit shared main or issue-305 workspace. Add focused positive cached and negative missing-toolchain checks without network. Run syntax and preflight manager-repro environment with PLAYWRIGHT_MODULE/CHROMIUM_PATH available; full signed three-tool browser gate waits Matrix #303/#304 integration and novidesign R6 release. Commit Sprint513 Story trailers, normal sprint submit under worker identity. Manager regrades, merges, accepts separately; no false live PASS.
