---
id: fefc62706f89
kind: bug
title: bashy make cannot run bashy's own Makefile (invalid inference rule; no --no-print-directory)
seq: 3167
status: todo
priority: p2
labels:
    - test
created: 2026-11-07T12:59:52.129471Z
---

Found 2026-11-07 (S586 gate on a Windows test host's podman machine, which has no GNU make). scripts/build-conformance-image.sh:55 calls 'make --no-print-directory test-bash-fixtures'. With bashy's in-process make on PATH: (1) 'make: unknown option --' for --no-print-directory; (2) without that flag, 'make: Makefile:1: invalid inference rule'. So make test-bash-container cannot run on a host whose only make is bashy's. Workaround used: a shim running the default test-bash-fixtures recipe verbatim (go run ./tools/bash53fixtures -root $PWD). Fix options: support GNU make's Makefile syntax used here + --no-print-directory in bashy make, or have the script call the fixtures tool directly. Classic-command change: check impact on the 86/86 and POSIX make gates; operator OK before touching coreutils.
