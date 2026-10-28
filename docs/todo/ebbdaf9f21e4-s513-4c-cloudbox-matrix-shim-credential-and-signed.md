---
id: ebbdaf9f21e4
kind: bug
title: S513.4c Cloudbox Matrix shim credential and signed-gate repair
seq: 2745
status: todo
priority: p0
labels:
    - deferred
    - operator-stop
    - operator-stopped
created: 2026-10-27T18:03:59.36545Z
weave: 304
---

Repair rejected bashy#301 commit 1bacfd4a; preserve author by fetching exact commit from issue-301 workspace into isolated child. In cloudbox_shim.py, ensure display-name update failure cannot make Cloudbox lose a newly generated one-time password or mistake completed reset; make safe cleanup logout non-masking. In cloudbox_gate.py, remove every test account using Provisioner.ref even on failure, and reject arbitrary MATRIX_CHAT_TUWUNEL binary override unless verified by signed Bashy admission. Add focused regressions for those cases. Rerun Python unit suite and native signed tuwunel protocol gate; only run production-vault gate after cleanup fix, on claimed host; await S514 R11 novidesign release for host use. Do not claim Cloudbox HTTPS three-token E2E PASS without real test service/tokens. Scope examples/matrix-chat and necessary docs only; no shared checkout reset/add-A. Normal sprint submit under worker identity; manager gates, merges, accepts separately.
