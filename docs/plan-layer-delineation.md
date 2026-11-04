# Plan: three-layer delineation (A Classic · B Bash# · C Yoke)

Status: planned 2026-11-04 (operator-approved direction), **deferred until the
Sprint 561 licensed run on candidate `f1bcd711` is complete**. Changing any
build input before then would void that run's evidence.

## Objective

A clear, enforced layering in the import graph:

| Layer | Contents | May import |
| --- | --- | --- |
| **A — Classic** | GNU Bash 5.3 / POSIX shell + coreutils (POSIX ∪ GNU coreutils ∪ `gencat`) — the POSIX-certified surface | A only |
| **B — Bash#** | Bash# / Bash++ language, Go-source front end, fenced runtimes (py/ts/rs/c/go/…) | A, B |
| **C — Yoke** | genie, software shaping, AgentOS, optional coreutils (external providers) | A, B, C |

The single `bashy` download still composes all three; the delineation is about
what each certified build *links*, so A's evidence is never voided by B or C
work.

## Evidence (Sprint 561 measurements)

- Linked packages: `./cmd/bash` 205 (13 `sh`, 0 coreutils); cert utilities
  build (`-tags=bashy_cert_base ./cmd/bashy`) 698 (17 `sh`, 177 coreutils,
  20 yoke, 2 bashsharp).
- Last 200 commits: `sh` 113 touch cert-linked source — 74 Bash++/Bash#-only,
  32 Classic-only, 5 mixed; coreutils 89 touch linked source, 111 do not.
- `sh/interp`: 211 of 355 non-test files are Bash++/Bash# code, linked
  unconditionally into `cmd/bash`.
- Coreutils is already layer A apart from `posixproviders` (optional providers
  `ar ctags ex nm strip vi`) and gate/benchmark tooling.

Boundary violations already recorded by the A1b audit
(`docs/s561/boundary-audit.md`): V1 `internal/cli` → `bashsharp/front`;
V2 `sh/interp` Bash++ bridge → `go/types` etc. in `cmd/bash`; V3
`internal/core` → `yoke/pkg/fleet` in the cert base; V4 cert base →
`bashsharp/transpile`.

## Stories (one per owning layer seam)

- **L1 — `sh`: Bash# engine out of `interp`.** Move the Bash++/Bash# engine
  files (and `gosource`/`lower`/`polyglot` reach) behind a registration seam so
  Classic `interp` links none of it (fixes V2). Owner layer B; touches A's
  engine, so it needs the full Classic gate before and after.
- **L2 — bashy CLI: Bash# front via injection.** Replace the direct
  `internal/cli` → `bashsharp/front` import with a hook set only by a
  Bash#-enabled entry point, as `AgentOSDispatch` already is (fixes V1).
- **L3 — cert utilities build: no yoke, no bashsharp.** Remove
  `yoke/pkg/fleet` from `internal/core` and `bashsharp/transpile` from
  `bashy_cert_base` (fixes V3/V4).
- **L4 — coreutils: optional providers to Yoke.** Move `posixproviders` and its
  manifest to yoke; coreutils `cmds` ⊆ POSIX ∪ GNU coreutils ∪ `gencat` plus
  gate tooling.
- **L5 — ratchets.** Per-layer import-boundary tests (extend
  `TestGoSourceFrontEndIsNotLinkedIntoClassicBash` and
  `TestCoreImportBoundary` to forbid every B/C package in A builds; a coreutils
  name-set ratchet) plus the Sprint 561 A5 content-addressed evidence identity
  reported per layer.

## Decision (operator, 2026-11-04): approved

Today `bash --bashsharp` / `--bashpp` works in the Classic `bash` binary
(since Sprint 97). Full delineation removes Bash# from `cmd/bash`; Bash#
remains available through `bashy` and the `bashsharp` binary. The operator
approved this user-visible change. L1/L2 must ship it with a clear error that
points `--bashsharp`/`--bashpp` users of `bash` to `bashy` or `bashsharp`, and a
changelog/release-notes entry.

## Sequencing and coordination

After the S561 licensed run and A4 closure. L4 and L5 are small and can go
first; L1 is the large, risky engine refactor and needs its own gates (Classic
86/86, internal/cli, cert closure digest change reviewed). Coordinate with the
Sprint 562 (layer B fence corpora) and 563 (layer C features) managers before
touching shared `sh`/bashy code.
