---
id: ab89b3bd5c84
kind: bug
title: 'bashy kubectl on Windows: MSYS-style /c/... path arguments (e.g. --kubeconfig /c/Users/.../tmp.X from mktemp) are not converted for the native kubectl, which then falls back to localhost:8080'
seq: 2724
status: done
priority: p0
created: 2026-10-24T20:11:55.678392Z
assignee: agy-gemini3.8-flash
sprint: 507
sprint_id: 77d7ab0f-6aef-5bd3-876d-3f50e9b585ea
sprint_title: 'YC demo blockers: four bug fixes (dks reattach, executor image default, AMD VRAM on Windows, kubectl /c/ paths)'
closed: 2026-10-26T06:08:55.9579Z
closed_by: s507-manager
---

Bug fix only (operator 2026-10-25: KISS). The whole fix is in internal/agentos/agentos.go where the kubectl and helm front doors call setCobraArgs(cmd, os.Args[2:]): on Windows (runtime.GOOS=="windows") pass args through mvdan.cc/sh/v3/pathconv.NativeArgs first, and convert KUBECONFIG, OUTPOST_KUBECONFIG_PATH, DKS_PEER_KUBECONFIG and DKS_FEDERATION_KUBECONFIG with pathconv.NativePathList (an earlier capped run, #284, had exactly this). Add ONE focused test of the argument/env conversion helper. Do NOT add a winpath command, atlas entries, registered.go changes or testdata baselines. Acceptance: focused test green; go vet; Windows cross-build; live check by s507-manager on elephant ('bashy kubectl --kubeconfig <mktemp path> get nodes').
