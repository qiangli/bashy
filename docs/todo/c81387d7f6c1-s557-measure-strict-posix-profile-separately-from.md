---
id: c81387d7f6c1
kind: test
title: S557 measure strict POSIX profile separately from Bash compatibility
seq: 3040
status: assigned
priority: p1
created: 2026-11-01T18:09:33.573856Z
assignee: s557-manager
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
---

Existing pinned Yash panel builds cmd/bash and invokes it as sh, intentionally exercising Bash-compatible POSIX behavior; its1778OK55ERROR are sharedwithBash5.3 and mustremainunchanged. Add explicitseparate strict cmd/sh profile measurement with clearlabel/provenance, samefixturepin/denominator/oracles and rawlogs. Never reinterpret Bashprofileerrorsasstrictcertification results. Keep defaultscript behavior and aggregatorcompatible. Public hostedcontainer execution only; boundedmock/syntax checks local. Licensedcertification is separate and private. Root ownsdispatch/review/pins.

Delivery: `YASH_PROFILE=strict scripts/yash-posix-suite.sh OUTDIR` selects
`./cmd/sh` and the `bashy-strict` label; panel names `alpine-strict` and
`debian-strict` keep every raw/verdict file distinct from existing measurements.
Unset or `bash-dropin` keeps `./cmd/bash`, `bashy`, and `alpine`/`debian` names.
The strict profile requires the retained Yash commit
0105ae707bce655034522372f8b2a20072819c28. Per-profile provenance records source
and fixture commits, executable hash, build target, argv0 sh, architecture,
and selected tests. Test selection, exclusions, thresholds, and oracle commands
are unchanged. Existing 1778 OK / 55 ERROR scores are not reclassified.

Bounded mocked OCI/compiler checks passed for default and strict target/panel
routing, distinct provenance, invalid-profile refusal, and wrong-fixture
refusal. Bash syntax and ShellCheck passed. No local foreign-suite execution,
container build, or product build occurred. Hosted strict measurement remains
pending manager dispatch and does not replace licensed certification.

Build-recipe follow-up: plain `bashy go build` selects Go but does not apply
the product runtime overlay. Both future profile measurements now use
`scripts/go-product.sh`, matching make build-bash/build-sh and the modernish
harness. This authenticates the pinned Go runtime source, applies the product
startup-signal contract, and enforces trimpath. Existing retained scores remain
historical observations from their original recipe; no scores are rewritten.
Provenance now explicitly records the driver, runtime contract, and complete
cross-build recipe. Mock verification exercised the real shell wrapper with a
fake Go launcher, asserting host bootstrap variables and preserved Linux/amd64
CGO=0 target arguments for both profiles. Syntax/ShellCheck passed; no actual
SDK overlay, build, container, or foreign suite was executed locally.
