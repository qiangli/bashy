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

## 2026-11-17: hit by the official build image

In the official build image (tools/bashy-build), commands run through bashy,
so `make build-bashy` reached bashy's in-process POSIX make and failed at once
with "make: Makefile:1: invalid inference rule" when rc.35 tried to build
rc.37. The build script now calls GNU make explicitly (/usr/bin/make), as the
convention for GNU Makefiles requires, so the release build is unblocked.
Raised to p1: bashy's make should hand a GNU Makefile it cannot parse to an
available GNU make (or name it in the refusal), not fail with a parse error.

Update the same day: the official build no longer uses make at all. bashy is
built by the previous bashy with its own `bashy dag build` target (Bash#), as
decided, so this is back to p2: the in-process make should still hand a GNU
Makefile it cannot parse to GNU make, but it no longer blocks any build.
