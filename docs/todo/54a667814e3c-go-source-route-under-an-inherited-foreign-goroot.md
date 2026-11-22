---
id: 54a667814e3c
kind: bug
title: 'Go-source route under an inherited foreign GOROOT: pinned toolchain compiles std with mismatched version'
seq: 3301
status: todo
priority: p2
labels:
    - gosource
created: 2026-11-22T14:07:27.588623Z
---

Found by P5 aa693f4a (2026-11-22). Root cause (recorded): the full cmd/bashy runs the Go-source route ('bashy --source=go DIR') through its pinned managed Go toolchain (go1.27.2) but leaves an inherited GOROOT in the child env. With GOROOT set to another toolchain (go test exports its own; users may set it too) the build fails: 'compile: version go1.27.4 does not match go tool version go1.27.2'. Reproduced on unmodified bashy main 42838686 (full binary) with GOROOT=$(go env GOROOT). Classification: product defect exposed by a realistic environment. Was masked because TestSourceRoute built the bashy_core probe profile, which resolved host go. P5 removed that profile; TestSourceRoute now strips GOROOT for its child (venue) and cites this todo. Fix where the cause lives: the managed toolchain (yoke/external/gotoolchain env or bash/gosource prepared_cache toolchainEnv) must pin GOROOT to the toolchain it runs.
