---
id: 1527e62f86de
kind: bug
title: Repair hosted Ollama runner compiler capture and Windows toolchain probe
seq: 1927
status: done
priority: p1
created: 2026-10-13T17:35:01.148855Z
weave: 101
assignee: codex-gpt6-astra
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T18:07:20.939971Z
closed_by: s436-codex-manager
---

Hosted diagnostic run 38439337620 (tag bashy-v0.21.2-1, producer 684032e) passed pin, then Linux amd64 build failed compiler dependency missing /tmp/cgo-gcc-input-*.c; Linux arm64 failed missing relative generated.c. Current capture resolves all relative deps against source root and reads them only after Go build, when cgo temp files are gone. Preserve full compiler invocation cwd and exact hash/content evidence for generated temporary compiler inputs at compile time; classify their trusted Go toolchain/generated origin narrowly, fail closed on unknown external native inputs, and retain exact non-system C/C++/Metal inventory. Windows job failed preflight test -x /c/msys64/mingw64/bin/gcc.exe; current GitHub windows-2025 image documents GCC and MSYS2 at C:/msys64, not this fixed path. Discover/verify GCC/G++ native target fail closed, record path/version. Arrange diagnostic compiler-input artifact upload even if later no-egress/audit fails, without permitting release unless all gates pass. Add focused adversarial tests for relative/temp inputs, compiler identity, diagnostic upload. Do not build/run native on Dragon. Commit worker-owned Bashy change with Sprint/Story/Story-ID trailers, no push; manager will review, integrate, install new branch descendant, tag next immutable revision.
