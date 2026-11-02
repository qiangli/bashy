---
id: a621f86d6a46
kind: task
title: S557 isolate signal artifact build gates within existing per-build deadlines
seq: 3043
status: assigned
created: 2026-11-02T02:48:08.962727Z
assignee: s557-public-tooling
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
---

Full hosted run41370653870 at umbrella58a2442 exhausted tools/elfaudit package default10m across four sequential real product cross-build/audit cases. Linux retained/stripped consumed about234s, Darwin product about278s, plain88s before aggregate kill. Stack is waiting on go build, not post-exit pipe drain. Preserve all four positive/negative assertions and exact architectures; isolate each existing subtest in its own bounded invocation rather than increase package/global timeout. Integrate into canonical make test with exhaustive named coverage and fail-fast mock checks. No full product builds on development host; root reviews/pushes.


Implementation: canonical `make test` now requires the isolated ELF/Mach-O
build-gate lane before its ordinary package sweep. The lane discovers both
existing top-level tests and runs Linux retained/stripped plus Darwin
product/plain in four separate test processes, each with the unchanged 10m
bound, exact selector, and count=1. Each JSON log must contain exactly the
requested successful subtest and a successful package result; zero selection,
renamed or skipped cases do not pass. Only those two already-executed top-level
tests are omitted from the subsequent broad sweep. Original test source,
recipes, architectures, positive assertions and stripped/plain rejection
checks are unchanged. Per-invocation logs are retained in the evidence tree.

Bounded local verification: shell syntax and diff whitespace checks passed.
Eight hermetic mock scenarios passed: four-case success, empty/missing
discovery, first-child failure with exact nonzero propagation and no later
child, missing/skipped/duplicate child results, and absent package completion.
No local product build or full suite was executed. Hosted full `make test`
remains required for final validation; this story is not accepted yet.
