---
id: 744d93b6177e
kind: bug
title: custom/kubectl adapter lost adaptKubeEnvironment (Windows KUBECONFIG path conversion) (P4 parity gap)
seq: 3304
status: assigned
priority: p1
labels:
    - bashy
    - windows
    - adapter
created: 2026-11-22T14:32:53.973317Z
assignee: codex-gpt6-astra
sprint: 692
---

Found by P5 aa693f4a (bashy 2d7f52ad left adaptKubeEnvironment in internal/agentos as the reference). Before the adapter conversion, 'bashy kubectl' on Windows converted KUBECONFIG paths/env for the kubectl child; custom/kubectl has no copy, so Windows users lose that conversion on main (regression vs rc.37). Fix: port adaptKubeEnvironment into custom/kubectl (duplicate per the independence rule), with a unit test for the Windows path case (build-tagged or table-driven on the pure function); then remove the dead in-process copy. TEST RULE (operator 2026-11-22): build + smallest targeted unit tests only, then submit; the one full run is the rc.40 candidate gate 458628c0. Start on current origin main; Sprint/Story/Story-ID + Agent trailers; bashy commit+push first, then the conductor bumps the umbrella pin. Owner codex, one run with the sibling stories filed at the same time (claim each).

Baseline investigation 25b60ada: the claimed missing Windows conversion is already wired in custom/kubectl/main.go through custom/internal/adapter/helpers.go (all four variables). No product conversion was missing. Moved the old dead-copy regression into custom/kubectl, exposed the platform branch for host-independent coverage, added path-list and non-Windows preservation checks, removed the dead agentos function. Targeted kubectl tests and dependency check PASS; native and Windows amd64 builds PASS. Native Windows execution deferred to candidate gate 458628c0.
