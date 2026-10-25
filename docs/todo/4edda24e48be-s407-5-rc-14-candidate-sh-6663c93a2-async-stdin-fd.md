---
id: 4edda24e48be
kind: task
title: 'S407.5 rc.14 candidate: sh 6663c93a2 (async stdin fd fix) + bashy carrier fd fix + signing prep; cut v1.0.0-rc.14-dev and re-run the Profile D replay'
seq: 1948
status: assigned
priority: p0
labels:
    - release
    - posix-cert
created: 2026-10-25T02:47:19.184344Z
assignee: s407-manager
sprint: 407
sprint_id: 90546935-fbc2-5d09-94b0-19c380846681
sprint_title: bashy 1.0.0 release gates pending external dependencies (follow-up to Sprint 379)
---

Manager integration story. Inputs: S407.4 9f3cc23e (bashy 5459ca41 carrier descriptor release; sh 6663c93a2 async stdin close, published to sh master 2026-10-25), S407.2 signing prep (815e0caa). Pin: bashy go.mod replace mvdan.cc/sh/v3 -> 6663c93a2 + srcModulePins. Gates: go build; internal/agentos pin tests; Linux fd regression (internal/cli TestCarrier, sh TestAsyncStdinDescriptorClosed) on Linux; make test-bash 86/86 (Linux evidence from S407.4 VM reused for identical code; macOS rerun on a remote Mac). Then tag v1.0.0-rc.14-dev, verify assets, new cert-approval/341-d tip, profile-d-do-repair + launch on droplet 612333931.
