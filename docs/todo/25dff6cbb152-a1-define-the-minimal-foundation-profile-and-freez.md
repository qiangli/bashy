---
id: 25dff6cbb152
kind: requirement
title: A1 Define the minimal foundation profile and freeze its baseline
seq: 3034
status: done
priority: p0
labels:
    - clean-baseline
created: 2026-11-04T02:24:39.236765Z
weave: 8
assignee: codex-gpt6-sol
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
closed: 2026-11-04T04:53:42.73741Z
closed_by: claude-opus5.5
---

Enumerate required shell/POSIX and promised GNU behavior, required core utilities, supported certification environment, authoritative suites and dependencies. Record current source/module/toolchain/provider/harness identities. Separate required acceptance from advisory probes. Adopt B -> A and C -> A/B with product-entrypoint composition; record actual violations, without broad refactoring. Acceptance: finite inventory and reproducible immutable baseline manifest; no legacy ticket reconciliation prerequisite.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

## Manager brief (2026-11-03, conductor claude-opus5.5)

Repo: umbrella/bashy, standalone module. ALL evidence and gates run with GOWORK=off; the umbrella go.work is inner-loop only and never evidence. Do not modify /Users/qiangli/projects/poc/dhnt (archived) or poc/bashy (stale clone).

Deliver under docs/s561/ (new dir) — reuse existing scripts; add at most one small manifest script:
1. `docs/s561/baseline-manifest.json` + the generator that produced it (prefer extending an existing scripts/ or `bashy mod` tool; else `scripts/s561-baseline-manifest.sh`). Record: bashy commit; sha256 of go.mod and go.sum; every require and replace with exact version; `go version` and toolchain line; CGO_ENABLED, build tags and flags used by `scripts/go-product.sh`; sha256 of bin/bash, bin/bash.real, bin/bashy, bin/bashy.real built by `GOWORK=off make build`; the pinned Bash 5.3 tarball SHA-256 and tools/bash53suite commit; container image digest for `make test-bash-container` if built; host OS/arch. Re-running the generator on the same commit must give identical non-host fields.
2. `docs/s561/inventory.md`: one finite table, each row = suite/lane, command, environment, containment, expected denominator, REQUIRED or ADVISORY, current-evidence pointer. Rows at minimum: GNU Bash 5.3 86-fixture gate (container lane authoritative); licensed 493-TP shell arm (Profile B); Profile D base utilities arm per the Sprint 341 base scope (102 mandatory targets = 92 Go applets + 10 Bashy shell commands, zero external providers, plus `gencat` in Go; the 14 optional targets are listed but do not gate — see docs/posix-command-coverage.md); `GOWORK=off go test ./...`; standalone build + 6-platform cross-build; product-wrapper build; advisory: yash, xcu/oils/austin/multishell diffs, zsh, uutils (contained only). The required profile is NOT an open decision (corrected 2026-11-03 by the operator; an earlier draft wrongly offered shell-only / full 116 / 84 Go-selected options from a stale doc): cite the base scope, add no utility and no external provider. Licensed runs are NOT authorized yet: mark them "blocked: awaiting operator approval".
3. `docs/s561/boundary-audit.md` is delivered by a separate worker — do not write it.

Acceptance: manager re-runs the generator and checks the manifest; inventory has no row without a command and denominator. Commit with trailers Sprint: #561, Story: A1, Story-ID: 25dff6cbb152. Do not push.
