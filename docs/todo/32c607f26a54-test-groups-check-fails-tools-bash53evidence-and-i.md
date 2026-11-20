---
id: 32c607f26a54
kind: chore
title: 'test-groups-check fails: tools/bash53evidence and integration/weavecli belong to no test group (pre-existing drift)'
seq: 3266
status: todo
priority: p2
created: 2026-11-20T20:52:54.571688Z
---

Found by S660 R3 (muse, 2026-11-20): 'bashy dag test-groups-check' fails identically on the untouched bashy checkout. New packages were added without a group in scripts/test-groups.sh. Assigning them is gate content (which CI lane runs them); decide the lane, then add. Not caused by R3.
