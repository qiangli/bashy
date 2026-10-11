---
id: 0334fb6a4183
kind: enhancement
title: Darwin bashy release builds without cgo (runtime overlay exposes inherited SIG_IGN snapshot)
seq: 1894
status: done
labels:
    - release
    - macos
created: 2026-10-11T21:58:21.499255Z
assignee: claude-opus5.5
sprint: 9999
sprint_id: 559b491b-4bab-57d3-bad4-5ceb9acbb9da
sprint_title: Maintenance
closed: 2026-10-11T23:44:04.192138Z
closed_by: claude-opus5.5
---

The only cgo in Darwin cmd/bashy is the pre-Go sigaction constructor in cmd/bashy/inherited_ignore_cgo_unix.go. Replace it with a push-linkname accessor added by the productgo runtime overlay (bitmask of runtime.fwdSig == SIG_IGN), behind build tag bashy_runtime_snapshot. Switch build-native-darwin-release.sh to CGO_ENABLED=0, invert elfaudit's Darwin check, update README and public-claims-evidence.
