---
id: 01a0be2a-6ea6-7255-a451-576bf005b255
seq: 2
form: page
type: gotcha
title: bashy Linux build is not static under CGO_ENABLED=0 (purego via coreutils locale gate)
description: 'When building a FROM scratch (or musl/Alpine) image with bashy, or claiming the lean cmd/bashy is static: on linux/{amd64,arm64} the CGO_ENABLED=0 binary still carries an ELF INTERP (/lib/ld-linux-*.so) and NEEDED libc/libdl/libpthread, because coreutils pkg/ctype and pkg/collate dlopen glibc through ebitengine/purego (fakecgo emits cgo_import_dynamic). A bare scratch image fails with "exec container process (missing dynamic library?)". Copy the ldd closure (examples/quickstart/stage-closure.sh) or build the image on glibc.'
tags:
    - scratch
    - container
    - purego
    - glibc
    - static
scope:
    os: linux
status: candidate
evidence: readelf -l /out/bashy in examples/quickstart/Containerfile builder stage, 2026-09-20, Sprint 216 Story 540; go list -deps shows github.com/qiangli/coreutils/pkg/{ctype,collate} importing github.com/ebitengine/purego on GOOS=linux
source:
    tool: claude-fable5.1-q
    host: dragon
    episode: weave-issue-43
created: "2026-09-20T09:34:07Z"
---

Symptom: a scratch image whose only file is the CGO_ENABLED=0 bashy exits 1 with "exec container process (missing dynamic library?) /bashy: No such file or directory".

Cause: purego (imported by coreutils pkg/ctype + pkg/collate on linux amd64/arm64 for the glibc-backed locale gate) uses fakecgo, which emits cgo_import_dynamic; the Go internal linker then produces a dynamically linked ELF even with cgo off. No build tag in coreutils or purego opts out.

Fix used: stage the four-file closure ldd reports (ld-linux, libc, libdl, libpthread; ~1.8 MB) beside the binary — examples/quickstart/stage-closure.sh. transpile --standalone binaries do NOT have this: they import only mvdan.cc/sh/v3 lower/shellrt, so mode-b is the one truly static artifact.

Looks right but: "CGO_ENABLED=0 therefore static" is false for this binary; verify with readelf -l | grep INTERP, not by reading the build flags.
