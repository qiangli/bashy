---
id: dd7899116e4f
kind: bug
title: TestEnsureBundledPodmanConf writes containers.conf into the real outpost install dir
seq: 3158
status: todo
priority: p2
labels:
    - test
    - windows
created: 2026-11-07T03:33:16.656354Z
---

Found on 2026-11-06 on the Windows test host. Running the native internal/... tests left a containers.conf in %LOCALAPPDATA%/outpost (the live install directory) pointing at a temporary test directory. Tests must write only under t.TempDir() or an overridden install root.
