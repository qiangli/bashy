---
id: 01a13724-c923-76b6-a74e-48660200d59f
seq: 6
form: page
type: gotcha
title: MinGW-clang Windows builds need -Wno-error downgrades for SOCKET/HANDLE int-conversion in pinned C deps
description: When a pinned C dependency passes Windows SOCKET (UINT_PTR integer) to HANDLE (pointer) parameters without a cast — e.g. nng win_tcpconn.c CancelIoEx(s, ...) — newer MinGW clang promotes -Wint-conversion to an error by default and fails the leg. Fix in the reviewed CMake overlay with target_compile_options(<dep> PRIVATE -Wno-error=<class>) inside if(WIN32), never by editing pinned source bytes; preempt the one-error-per-run loop by covering sibling default-error classes (-Wincompatible-pointer-types, -Wimplicit-function-declaration) in the same guarded block, and note C++ targets need no equivalent since these are hard errors in C++.
status: candidate
source:
    tool: muse-spark1.3-i
    host: dragon
    episode: weave-issue-113
created: "2026-10-13T21:22:00Z"
---
