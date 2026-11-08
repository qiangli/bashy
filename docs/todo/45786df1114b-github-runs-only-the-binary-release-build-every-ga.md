---
id: 45786df1114b
kind: enhancement
title: GitHub runs only the binary release build; every gate runs on the local fleet in under 30 min
seq: 3196
status: todo
priority: p1
labels:
    - release
    - ci
created: 2026-11-08T06:41:29.546581Z
sprint: 585
sprint_id: 73032105-b38f-5cd7-8f47-a732dfee103f
sprint_title: Klatch v1.2.7 rebuild and end-to-end tests
---

Operator rule (2026-11-08): only the binary release build runs on GitHub; everything else runs locally.

Scope:
1. release.yml keeps only build + sign + publish. Remove license-gate and govulncheck as build prerequisites (release.yml:98,188,270 `needs:`); run them on the fleet before the -dev tag and record the evidence with the candidate. Turn off setup-go cache save (3.5-9 min per job, always misses) and the duplicate Go setup. Build otel alongside goreleaser.
2. Local gate lanes under 30 min: keep a go.sum-keyed GOMODCACHE/GOCACHE volume instead of deleting it (test-self-container.sh:41; warm-up 13+ min today); shard `go test ./...` into capped containers across puppy and elephant (desk | agentos | elfaudit+productgo | cmd/bashy+rest | pre-targets + test-bash-container), -timeout 45m, with a check that the shard union equals `go list ./...`; build each bashy variant once per lane.
3. Move airgap, conformance, quickstart-scratch and install-matrix checks to local lanes (their workflows were disabled 2026-11-08). Fix the stale docs/airgap-image.md rows (sf, slack) found on the rc.28 tag.
Acceptance: an rc whose only GitHub job is the release build, with local gate evidence for every removed check and a measured local wall clock under 30 min.

## Test groups (operator 2026-11-08: do this now, not after rc.28)

Only 13 packages (+ cmd/bash, cmd/sh) link into the shipped binaries
(`go list -deps ./cmd/bashy`). Groups, with S586 puppy timings:

| Group | Packages | Time | Runs when |
|---|---|---|---|
| 0 deps | sh, bashsharp, yoke, coreutils, outpost | own repos | gated once per pinned SHA in its own repo; bashy checks evidence exists |
| 1 core (shipped) | dhnt, skills, internal/{tmpl,httpproxy,installpair,transcribecontract,interactiveprobe,core,transcribeseed}, internal/agentos/{activity,session}, pkg/{ladder,harnessrunner,runner} | ~1 min | inputs changed (content-keyed reuse otherwise) |
| 2 agentos (shipped) | internal/agentos | 554-600s | inputs changed; own shard |
| 3 front end (shipped) | internal/cli, cmd/bashy, cmd/bash, cmd/sh | 278s + 354-600s | inputs changed; binary-building tests move to 6 |
| 4 release tooling | tools/* except elfaudit/productgo, scripts, native | ~1.5 min | tooling changed; candidate for its own module |
| 5 example apps | examples/* (desk 849s, outpost-profile 186s, ...) | ~17 min | app changed; becomes separate modules pinning a released bashy |
| 6 binary e2e | bash53 86 fixtures, cmd/bashy startup/inherited-ignore, tools/elfaudit, tools/productgo, tier smoke | ~25 min serial | every candidate, always fresh (-count=1) |

Rule: a group union check must equal `go list ./...` so no package is silently dropped;
reused results are labelled reuse (content key), fresh runs labelled fresh.
