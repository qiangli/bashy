---
id: b24f3f952e32
kind: bug
title: a dir-sourced genie bundle (bashy genie build --from a workspace) replaces the host bundle and never refreshes from the builtin source; make install also skips the rebuild when only sibling trees change
seq: 423
status: todo
priority: p2
labels:
    - genie
created: 2026-10-09T15:36:02.368322Z
sprint: 379
sprint_id: 908c2ac2-e7bc-55fe-86bd-d25046ac4684
sprint_title: 'bashy 1.0.0 feature list: bash + Bash# + Yoke'
---

S379 2026-10-09: a ycode weave worker ran its own genie smoke, which ran bashy genie build from its workspace (examples/genie); ~/.bashy/genie/genie.source.json became {source: dir, path: <that weave workspace>} and every later bashy genie run on the host used that worker's bundle, even after the installed bashy carried newer builtin genie source (live symptom: project-instructions fragment 0 bytes until the conductor ran bashy genie build). Fix: a bundle built --from a directory is a per-invocation or per-workspace override, never the host default (or record it and rebuild from builtin when the builtin digest changes and the recorded dir no longer exists or is not the caller's). Second, related: bashy make install keeps bin/bashy when only sibling module trees (go.work members ycode, yoke, sh) changed, so a reinstall after a sibling fix silently installs the old binary; make the install target rebuild when any go.work member changed (or always build; go's cache makes it cheap).
