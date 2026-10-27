---
id: 2fd63b2c14b9
kind: bug
title: S513.4d Cloudbox Matrix gate read-only signed binary preflight
seq: 2747
status: todo
priority: p0
created: 2026-10-27T18:14:53.521055Z
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
---

Manager review of rejected bashy#304 7650c506: cloudbox_gate.py tuwunel_binary() delegates to provision.installed_binary(), which runs bashy tool install tuwunel and can fetch bytes during automated E2E. Repair only the gate preflight (and focused test/docs) to use a read-only Bashy tool show/metadata lookup plus installed-file digest verification against the platform asset; fail closed if absent or mismatched, never install/download. Preserve exact #301 author and #304 repair commits by fetching their branch from issue-304 into a new isolated workspace; do not edit shared main/issue-304 workspace. Verify Python unit suite and a negative missing/unsigned override test, then actual signed tuwunel protocol gate when authorized host available. S514.R6 currently has bounded novidesign slot; wait for DONE before host use. No Cloudbox HTTPS three-token PASS without real test service/tokens. Normal Sprint513 Story trailers and sprint submit under worker identity. Manager gates/merges and accepts separately.
