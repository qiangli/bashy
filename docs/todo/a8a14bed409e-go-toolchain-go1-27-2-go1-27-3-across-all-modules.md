---
id: a8a14bed409e
kind: task
title: Go toolchain go1.27.2 -> go1.27.3 across all modules (GO-2026-6809 crypto/x509)
seq: 3160
status: todo
priority: p1
labels:
    - toolchain
    - security
    - release
created: 2026-11-07T07:36:49.624273Z
---

Why: govulncheck finding GO-2026-6809 (stdlib crypto/x509 must distrust compromised intermediate certificates) is reachable in bashy and fixed in go1.27.3. It is report-only for v1.0.0-rc.27 and listed in its known-issues ledger.
This is NOT a one-line bump:
1. tools/productgo hard-requires exactly go1.27.2 (main.go ~102). It builds the product runtime that exposes the Go runtime's inherited-ignored-signal snapshot to pure Go (Darwin CGO_ENABLED=0, POSIX SIG_IGN). Port that hook to go1.27.3, or re-validate it.
2. Move the toolchain line together in bashy, yoke, sh, coreutils, outpost and bashsharp, plus the Go baseline in umbrella CLAUDE.md and docs/build.md, so GOWORK=off standalone builds stay consistent. Leave third-party builds/*/fork workflows alone unless their own tool requires it.
3. Gates on the Windows test host and the release QA hosts: strict make test-bash-container 86/86, the inherited-signal tests (internal/cli signal tests, scripts/verify-bashy-signal-artifact.sh, tools/bashysignalprobe on darwin), the native tier smoke, and the licensed TP164 re-run on the licensed host.
4. Report the certification-input change to S571 for evidence-identity revalidation.
5. Ship it in the next release candidate.
Status: filed 2026-11-07 on operator request. DO NOT START step 1 until the operator says so.
