# A1b — Boundary audit for the Classic certification graph

Sprint 561, story A1b (`9a54012155dd`), separate-owner slice of A1
(`25dff6cbb152`, acceptance unchanged). Audit only: **nothing was fixed here.**
Any repair belongs to A3 (`3d4d3d262bdd`) and only if it blocks the base
profile.

- Baseline audited: `bashy` main `5a4bd03f` (current checkout `866c67f7`, which
  is `5a4bd03f` plus docs-only commits — `git diff 5a4bd03f HEAD -- . ':!docs'`
  is empty).
- Module mode: standalone, `GOWORK=off` (the umbrella `go.work` is inner-loop
  only and is never evidence).
- Toolchain: `go version go1.27.2 darwin/arm64`.
- Not touched: `$HOME/projects/poc/dhnt`, `poc/bashy` (both out of scope per the
  card).

## 1. Layering contract being audited

From `docs/plan-clean-baseline-campaign.md` (§Direction):

> Use B -> A and C -> A/B, with composition at the product entrypoint. A owns
> shell behavior and required core utilities; B owns fence/language contracts;
> C owns optional providers and Yoke/product integration.

So the permitted dependency directions are `B -> A` and `C -> A/B`; a violation
is any edge in the opposite direction — an **A-layer package importing a B- or
C-layer owner** — that reaches the Classic product graph.

Owners as they exist in the module graph:

| Layer | Owning module(s) | Role |
| --- | --- | --- |
| A (shell + required coreutils) | `github.com/qiangli/bashy` (`internal/cli`, `cmd/bash`, `internal/core`), `github.com/qiangli/coreutils`, `github.com/qiangli/sh/v3` (via `replace mvdan.cc/sh/v3`) | shell behavior and the Sprint 341 base pure-Go utilities |
| B (fence/language contracts) | `github.com/bashsharp/bashsharp` (Bash#/Go-source language front), language/fence corpora | the language the shell runs |
| C (optional providers, product integration) | `github.com/qiangli/yoke`, `github.com/qiangli/ycode`, `github.com/filebrowser/filebrowser/v2`, `github.com/dhnt/dhnt`, `github.com/modelcontextprotocol/go-sdk`, `modernc.org/sqlite`, `github.com/ollama/ollama` | AgentOS/product surface |

The **Classic product graph** is the certification subject — the shell plus the
Sprint 341 base pure-Go utilities (`docs/posix-command-coverage.md`, 102
mandatory targets = 92 Go applets + 10 shell commands, zero external providers).
Two entry points serve it:

- `./cmd/bash` — the pure GNU Bash 5.3 drop-in (the shell arm).
- `./cmd/bashy` — the multicall carrier for the Go applets; its certification
  form is the `bashy_cert_base` build tag (also selected by the
  `test-startup-linux` gate with `-tags bashy_cert,bashy_cert_base,osusergo`),
  which exists precisely to keep optional packages out of the base product.

## 2. Existing ratchets used (no new tool)

Three in-repo ratchets already pin this boundary; all were run and pass:

1. `TestGoSourceFrontEndIsNotLinkedIntoClassicBash`
   (`internal/agentos/gosource_agentos_test.go`) — asserts `cmd/bash` links
   neither `mvdan.cc/sh/v3/gosource` nor `internal/agentos`, and asserts (as a
   corrected, observed fact) that `cmd/bash` *does* link `go/types`. Result:
   `go test ./internal/agentos/ -run TestGoSourceFrontEnd` → **PASS**.
2. `TestCoreImportBoundary` (`cmd/bashy/core_imports_test.go`, build tag
   `bashy_core || bashy_cert_base`) — asserts the tagged base graph contains no
   `internal/agentos`, `ycode`, `filebrowser`, `yoke/pkg/telemetry` or
   `modernc.org/sqlite` package. Result:
   `go test -tags bashy_cert_base ./cmd/bashy/ -run TestCoreImportBoundary` →
   **PASS** (both subtests).
3. `TestBashyBinaryDoesNotLinkOutpost` (`internal/agentos/outpost_closure_test.go`,
   itself `GOWORK=off`) — asserts the `./cmd/bashy` graph contains no outpost or
   yamux package. Result: PASS.

Also passing (same package run): `TestGoSourceFrontEndIsLinkedByDefault`,
`TestClassifyLicense`/SBOM checks. Upstream ratchet
`bashsharp/front.TestBashPPImportsOnlyTheEngine` pins the B→A floor
(`sh ← bashpp ← bashy`, no `bashy`/`coreutils`/`yoke` imports inside bashsharp).

### Recipe (recorded, not scripted)

No new tool was added; the audit used, verbatim:

```sh
GOWORK=off go list -deps ./cmd/bash                      # Classic shell graph
GOWORK=off go list -deps ./cmd/bashy                     # full product graph
GOWORK=off go list -deps -tags bashy_cert_base ./cmd/bashy   # Classic base profile
GOWORK=off go list -deps -json <target>                  # per-package Imports for reachability
grep -rln 'qiangli/yoke\|qiangli/ycode\|filebrowser\|modelcontextprotocol' \
  --include=*.go internal cmd pkg dhnt                    # direct A→B/C edges
go test ./internal/agentos/ -run TestGoSourceFrontEnd -v   # ratchet 1
go test -tags bashy_cert_base ./cmd/bashy/ -run TestCoreImportBoundary -v  # ratchet 2
```

Package counts: `./cmd/bash` 205, `./cmd/bashy` (default) 2113,
`./cmd/bashy` (`bashy_cert_base`) 698.

## 3. Violations found

Every row below is an **actual observed edge** in the current standalone graph,
with its owning module, whether it is linked into the Classic binary (i.e.
reaches `./cmd/bash`, or `./cmd/bashy` under `bashy_cert_base`), and whether it
blocks the base profile (the certification claim of §1).

| # | Violating edge (A → B/C) | Owning module | In Classic binary? | Blocks base profile? |
| --- | --- | --- | --- | --- |
| V1 | `internal/cli` → `github.com/bashsharp/bashsharp/front` (Bash#/Go-source selection, `front.ResolveBashPP`, `front.StripGoSourceInvocationFlags`, `front.GoSource*` hooks: `internal/cli/main.go`, `internal/cli/session.go`, `internal/cli/gosource_startup.go`) | `github.com/bashsharp/bashsharp` (B) | **Yes — `cmd/bash` and `cmd/bashy` (both profiles)** | **No.** Documented and deliberate: `front`'s Go-source hooks are nil in the drop-in, `internal/cli` is the shared entry point, and `go list -deps ./cmd/bash` shows no `mvdan.cc/sh/v3/gosource` and no `internal/agentos`. The Bash# *front end* itself stays in the B/A-consumer half — exactly what `TestGoSourceFrontEndIsNotLinkedIntoClassicBash` pins. |
| V2 | `mvdan.cc/sh/v3/interp` → `go/types`, `go/importer`, `go/build`, `go/format`, `go/parser` (Bash++ native bridge, `interp/bashpp_native_bridge.go` and 20+ `bashpp_*`/`gosource_*` files; imported unconditionally, no build tag) | `github.com/qiangli/sh/v3` (A engine, hosting B's language) | **Yes — `cmd/bash` and `cmd/bashy` (both profiles)** | **No (behaviorally), recorded as a known size/type-checker leak.** The pure `bash` drop-in carries the Go type checker and ~half of `go/*` even though it never runs Bash++. The repository ratchet *asserts* this state (`TestGoSourceFrontEndIsNotLinkedIntoClassicBash` fails if `go/types` disappears, so the doc comment is not "fixed" back), and the drop-in's own doc (`cmd/bash/main.go`) still claims "links no coreutils or external-tool wiring" — true — while `front/gosource.go`'s older "cmd/bash links no go/types" claim is stale by record. Does not gate certification: no behavior or provider dependency is introduced. A size/binary-hygiene fix is A3 material, not a blocker. |
| V3 | `internal/core` → `github.com/qiangli/yoke/pkg/fleet` (command CRUD catalog; transitively pulls 20 yoke packages incl. `pkg/assetring`, `pkg/binmgr`, `pkg/broker/door`, `pkg/provider/*`, `pkg/room`, `pkg/secrets`, `pkg/shape/appserver`, `pkg/spacetime`, `yoke/git`) | `github.com/qiangli/yoke` (C) | **No for `cmd/bash`.** **Yes for the base profile:** `bashy_cert_base` (and `bashy_core`) link `internal/core` → `yoke/pkg/fleet`. | **No.** Known and intentional: `internal/core`'s package doc says the core route "deliberately has no AgentOS, ycode, or Genie imports" — `yoke/pkg/fleet` is the *catalog* primitive, and `TestCoreImportBoundary` explicitly allows `yoke/pkg/fleet` while forbidding `internal/agentos`, `ycode`, `filebrowser`, `yoke/pkg/telemetry`, `modernc.org/sqlite`. The 92 applets themselves (`coreutils/cmds/*`, 177 packages) import no yoke/ycode/bashy package at all (verified: zero hits). If the profile is later required to drop the fleet catalog, that is a C-consumer decision for A3, not a certification blocker. |
| V4 | `cmd/bashy` (base profile) → `github.com/bashsharp/bashsharp/transpile` → `mvdan.cc/sh/v3/gosource`, `bashsharp/front` → `go/types` | `github.com/bashsharp/bashsharp` (B) | **Yes for the base profile** (`bashy_cert_base`/`bashy_core` import `_ "github.com/bashsharp/bashsharp/transpile"` to register `front.GoSourceLoad` without AgentOS); **No for `cmd/bash`.** | **No.** The base entry point's comment states the Bash# Go-source loader "is part of the language base"; `TestCoreImportBoundary` does not forbid it. |
| V5 | `cmd/bashy` (full) → `github.com/qiangli/yoke/pkg/telemetry`, `yoke/pkg/shape/boundary`, `ycode/pkg/ycodecli` (direct imports in `cmd/bashy/main.go` + `app_boundary.go`) | `github.com/qiangli/yoke`, `github.com/qiangli/ycode` (C) | No (full profile only; `bashy_cert_base` selects `main_core.go` instead of `main.go`) | No — the full product binary is a C composition point by design ("composition at the product entrypoint"). |
| V6 | `internal/agentos` → 150+ `yoke` packages, `ycode/examples/genie`, `dhnt/dhnt/skills`, `modelcontextprotocol/go-sdk/mcp`, `filebrowser/v2` (via `go.mod` replace) | C owners | No (`bashy_cert_base` graph contains no `internal/agentos` package at all) | No — AgentOS is the C half; the base profile excludes it, as pinned by ratchet 2. |
| V7 | `bashy/dhnt` (bashy module) → `github.com/qiangli/yoke/pkg/dag`; transitively ~44 yoke packages incl. `pkg/telemetry`, `pkg/provider/*`, `pkg/secrets` | `github.com/qiangli/yoke` (C) | No — reached only via `internal/agentos` → `bashy/dhnt` (V6 path) in the full binary | No (not in the Classic graph). Noted because it is an A-module package importing C directly; if `dhnt` is ever promoted into the base profile it would become a blocker and should be re-audited. |
| V8 | `bashy/pkg/harnessrunner` → `yoke/pkg/atlas`, `yoke/pkg/kb`, `yoke/pkg/scope`; `bashy/pkg/ladder`, `bashy/pkg/runner` (→ `internal/agentos`) exist in the module | `github.com/qiangli/yoke` (C) | No — reached only through `internal/agentos` in the full binary | No (harness/agent tooling, not the product graph). |
| V9 | `internal/agentos/activity` → `yoke/pkg/bus`; `internal/agentos/session`, `internal/httpproxy`, `internal/installpair`, `internal/tmpl`, `internal/transcribecontract`, `bashy/skills` | `github.com/qiangli/yoke` (C) | No | No (all inside the AgentOS/full-product half). |

### Clean directions confirmed (no violations)

- `cmd/bash` links **no** `coreutils`, `yoke`, `ycode`, `dhnt`,
  `internal/agentos`, `internal/core`, `bashy/dhnt` or `bashy/pkg/*` package:
  its bashy-module packages are exactly `internal/cli` and `cmd/bash`. This is
  the Classic shell arm — clean.
- The 92-applet set (`coreutils/cmds/**`, `coreutils/tool`, `coreutils/shell`,
  177 packages in the base graph) imports **zero** B/C-owner packages — the
  utilities layer is clean in both the full and `bashy_cert_base` graphs.
- `coreutils` imports nothing from `bashy`, `yoke` or `ycode` anywhere in the
  graph (A stays at the bottom), and `bashsharp` (B) imports nothing from
  `bashy`/`yoke` (pinned by `TestBashPPImportsOnlyTheEngine`); its only
  `qiangli/` reference is a generated `go.mod` string in
  `transpile/transpile.go`.
- `yoke`/`ycode` (C) import `coreutils` packages (`pkg/lockfile`, `pkg/weavecli`,
  `pkg/schedule`, `tool`, `shell`, `cmds/all`) — that is the permitted C→A
  direction, not a violation.

## 4. Verdict

**No B→A/C→A/B dependency violation blocks the Classic base profile.** The
boundary holds where it must: the pure shell binary stays free of the AgentOS,
coreutils-applet, provider and product-integration graphs, and the tagged base
profile carries shell + 92 Go applets + gencat with no AgentOS/ycode/
filebrowser/sqlite/outpost packages.

Two recorded non-blocking deviations remain, both already pinned by ratchets and
both candidates for A3 only if the profile ever demands them:

1. **V2** — the `go/types`/`go/importer`/`go/build` subtree is linked into the
   pure `cmd/bash` drop-in through the sh fork's unconditional Bash++ bridge
   (size/binary hygiene; ~100 packages of the 205-package graph).
2. **V3/V4** — the base profile's `internal/core` keeps `yoke/pkg/fleet` (20
   yoke packages) and `bashsharp/transpile` (B's Go-source loader) inside the
   certification candidate. Both are permitted by the base-profile ratchet;
   neither is a provider or an external utility, so the zero-provider,
   pure-Go claim of `docs/posix-command-coverage.md` is unaffected.

Fixes intentionally not made here (per card): the stale half of the
`cmd/bash`/`front` doc claims, any `go/types` decoupling in the sh fork, and any
catalog/loader relocation out of `internal/core`. Route them to A3 with a
red/green case only if a profile requirement turns them into blockers.
