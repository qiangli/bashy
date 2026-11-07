---
id: 7cccee9ac6aa
kind: bug
title: bashy self install --force refuses to replace outpost with the same version
seq: 3155
status: todo
priority: p2
labels:
    - install
    - outpost
created: 2026-11-07T02:59:21.869924Z
---

Observed 2026-11-06 installing v1.0.0-rc.27-dev on the Windows test host. After outpost alone had been upgraded to rc.27-dev, 'bashy self install --force' refused to replace the rc.27 outpost with the same rc.27 build, so the documented full install (bashy, bash, outpost together) could not proceed until 'outpost rollback' restored rc.26. Expected: --force reinstalls even when the versions match, or the error explains which component blocks and how to proceed. Repro: upgrade outpost to version X alone, then run bashy self install --force with release X.
