---
id: bb0b062f6b8b
kind: bug
title: S557 build and preflight portable public conformance artifacts
seq: 3035
status: done
priority: p1
created: 2026-11-01T16:55:21.123166Z
assignee: s557-public-tooling
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
---

Free POSIX CI41327538807 produced bogus corpus deviations because posix-diff.sh built cmd/bash with inherited CGO1 then mounted the glibc-dependent binary into Alpine/musl. Use the existing product build wrapper and CGO_ENABLED=0 for portable Linux conformance artifacts; verify testee executes in each image before counting assertions. Preserve corpus, denominators, comparison semantics and raw failures. Audit same artifact prerequisite in related public runners. Short mocked builder/preflight checks local; actual suites remote only.

Delivery: posix-diff, modernish and dash producers use the product runtime build
wrapper with CGO disabled. Each executes the mounted testee inside its panel
image under a 10-second timeout before any assertions; failed startup exits 2
without reporting corpus measurements. Corpus contents and comparison rules
are unchanged.

Validation: bash syntax and diff checks passed. Six isolated mocked checks
verified Linux/amd64 CGO0 product builds, successful startup reaching the corpus,
and startup exit127 returning2 before corpus execution across all three scripts.
Actual container suites remain remote validation work.
