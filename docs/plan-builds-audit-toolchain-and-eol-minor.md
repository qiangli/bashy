# builds: an audit toolchain of its own, and the EOL-minor security override

Sprint 434, story #1902. Follows the second-forge/module-override story, which
landed `GO_SECURITY_OVERRIDES`, `UPSTREAM_TAG_FORM` and the non-GitHub forge.
Scope: `builds/` only; `builds/_shared` is owned by this story.

## The three problems

**1. The audit tools cannot be built with the tool's toolchain.** The `go`
kind installed `govulncheck` and `cyclonedx-gomod` under the same
`GOTOOLCHAIN` as the artifact. `govulncheck` v1.9.0 is a Go module whose own
`go.mod` says `go 1.26.0`, so on a tool pinned to anything older the `go
install` fails before a single package is analysed — which is how
`qiangli/kopia` run 38326642419 died on kopia v0.23.1's go1.25.11. The two
versions have nothing to do with each other and must not be coupled.

**2. A pin on an end-of-life Go minor cannot be made safe.** Go ships security
fixes for its two newest minors only, so go1.27 shipping retired 1.25:
go1.25.11 is the last 1.25 release there will ever be. The toolchain gate
required `GO_TOOLCHAIN_SECURITY_OVERRIDE` to stay on the upstream minor and be
a later patch — which, on a retired minor, names nothing. kopia v0.23.1 had no
move available.

**3. `govulncheck ./...` cannot answer the release question.** `./...` makes
every package in the repository a root — test helpers, and the extension
packages a reduced build deliberately keeps out of the binary. Build tags do
not fix it: zot's `pkg/extensions/search/cve/trivy` carries no build constraint
at all, so `./...` reports trivy's unfixed `GO-2026-4919` as reachable in a
binary that links none of it. The only ways out would have been a waiver (never,
for a reachable finding) or refusing to ship a tool that is not vulnerable.

## The shape

### `GO_AUDIT_TOOLCHAIN` — the audit tools' own pin

`builds/render.sh` declares it once (`1.27.2`), overridable per tool.
`_shared/kinds/go/audit.yml` installs both tools with
`GOTOOLCHAIN=go<GO_AUDIT_TOOLCHAIN>` and nothing else does. `go install
pkg@version` resolves in module-agnostic mode and never reads `src/go.mod`, so
it cannot reach the module being judged; and the **analysis** still runs at the
module's effective toolchain, because `govulncheck` shells out to `go` and
`GOTOOLCHAIN` in the environment pins that to what the artifact is built with.

`audit.yml` now `@@INCLUDE setup@@`s the build job's setup rather than keeping a
verbatim copy of it, so "the audit judges the same overridden tree the build
compiles" is structural instead of a convention.

### EOL-minor security override

`GO_SUPPORTED_MINORS` (also in `render.sh`, oldest first) is the one place the
supported set is written down. The toolchain gate now accepts exactly two
shapes, and in both the override must be strictly later than `GO_TOOLCHAIN` —
a downgrade or a no-op is always refused:

- **in-minor patch bump** — a later patch of the tag's own minor. The only
  shape allowed while that minor is still supported.
- **EOL-minor security override** — allowed only when the tag's minor is *not*
  in `GO_SUPPORTED_MINORS`, and only to the **oldest** supported minor's latest
  patch: the smallest language-version jump that gets fixes again.

Refused: a jump while the tag's minor is still supported, and a jump to
anything but the oldest supported minor. The step logs the label, `PIN` carries
the key (so the `<tool>-pin.txt` asset records it) and `RELEASE_GATES` names it
in the release notes as an "EOL-minor security override".

### The go/toolchain directive gate

Taking x/net v0.60.0 or x/crypto v0.56.0 — both declare `go 1.26.0` — into an
older module forces `go get` to raise `go.mod`'s own `go` line, which the
module gate previously refused outright, making those fixes unreachable. The
`go` directive may now rise, but **never past the effective build toolchain**,
which the toolchain step has already gated. That cap is the whole safety
argument: under an in-minor override it confines the language version to the
tag's minor, and under an EOL-minor override it permits exactly the move that
override already sanctioned. A directive that drops, or one above the effective
toolchain, stays refused; the `toolchain` line may only be dropped or lowered,
since `GOTOOLCHAIN` is what selects the release.

### `GO_AUDIT_TAGS` and the audit scope

`govulncheck` and `cyclonedx-gomod` now run over `GO_PACKAGE` with
`GO_AUDIT_TAGS` and `GO_BUILD_ENV` — the shipped build configuration — so
"reachable" means reachable in the signed artifact and the SBOM's component
list is the artifact's. `render.sh` requires `GO_AUDIT_TAGS` as soon as
`GO_BUILD_TAGS` is non-empty, even to say "none": the audit configuration is
never a default. The one sanctioned divergence is a tag needing generated
sources the audit job does not produce (gitea's `bindata`).

### `_shared/kinds/go/toolchain-gate-test.sh`

Both gates live in `run:` blocks that only execute on a release runner, so a
sibling of `egress-verdict-test.sh` extracts them from `setup.yml` (BEGIN/END
markers delimit the directive half) and drives 19 synthetic cases. `check.sh`
runs it. Verified mutation-sensitive: inverting "oldest supported" to "newest
supported" fails two cases.

## Declarations, each measured locally at the pinned commit

`govulncheck` v1.9.0 built with go1.27.2, run at each tool's effective
toolchain, over its `GO_PACKAGE` and audit tags. **0 reachable, 0 waivers** for
all three; every `vuln-ignore` stays empty.

| tool | tag | toolchain | `GO_SECURITY_OVERRIDES` | reachable |
|---|---|---|---|---|
| gitea | v28.1.0 | go1.27.1 -> **1.27.2** (in-minor) | `golang.org/x/net@v0.60.0` | 0 |
| zot | v2.1.22 | go1.27.0 -> **1.27.2** (in-minor) | x/net v0.60.0 + otlptracegrpc/otlptracehttp v1.45.0 | 0 |
| kopia | v0.23.1 | go1.25.11 -> **1.26.9** (EOL-minor) | 23 modules, derived from five fixes | 0 |

**gitea** — `qiangli/gitea` run 38326618659 measured 12 reachable stdlib
findings at the tag's go1.27.1 plus 5 in x/net v0.59.0. Both overrides clear
them and nothing else in the graph moves.

**zot** — run 38326659679 refused the audit with three findings that have **no
upstream fix**: `GO-2026-4919` (trivy), `GO-2026-6225`
(docker-credential-acr-env) and `GO-2026-5932` (x/crypto/openpgp, deprecated
and frozen). Each arrives through one extension build tag, and each extension
is already disabled at runtime by the empty `extensions` map in
`files/config.json` — so `GO_BUILD_TAGS` drops `search`, `sync` and
`imagetrust` on top of `ui`, leaving
`events,lint,metrics,mgmt,profile,scrub,userprefs`, and `buildinfo.BinaryType`
is stamped `bashy-minimal`. With the audit scoped to `./cmd/zot` and the same
tags, all three become module-level only. `buildinfo.GoVersion` now follows
`${GO_TOOLCHAIN_SECURITY_OVERRIDE:-$GO_TOOLCHAIN}`, so the override cannot
leave the running binary reporting a Go release it was not built with.

**kopia** — the EOL move to go1.26.9 clears every reachable stdlib finding on
its own, leaving 11 in dependencies, all with published fixes: x/net v0.60.0,
x/crypto, grpc v1.83.1, x/text, and the otel OTLP trace exporter v1.45.0.
Taking those into a graph this old moves 23 require lines by MVS and raises the
`go` directive from 1.25.8 to 1.26.0, so the declaration names every module
that moves, each at the version MVS selects — pinning the *selected* version is
what keeps the extra entries no-ops rather than downgrades (naming
`otel/sdk@v1.45.0` in zot's list would have downgraded it from the v1.46.0 the
tag picks). The list is derived; regenerate it when the pin moves.

### kopia's update check is an envar, not a flag

`--[no-]check-for-updates` is declared by the repository *connect* options
(`cli/command_repository_connect.go` at the tag), so it only parses after a
subcommand that installs them. The no-egress gate passed it in the **global**
position on both invocations, where it is an "unknown long flag" that kills the
process — which is why the gate failed. Verified against a binary built from
the pinned commit: `kopia --no-check-for-updates server start` exits 1,
`KOPIA_CHECK_FOR_UPDATES=false kopia … repository create` and `… --otlp-trace
server start` both exit 0. So `files/no-tracking.env` sets
`KOPIA_CHECK_FOR_UPDATES=false` (the exact `checkForUpdatesEnvar` from
`cli/update_check.go`, with no prefix — `envNamePrefix` is test-only), the flag
is gone from the gate, the README, `otel.env` and the `RELEASE_NOTE`, and
`--otlp-trace` stays a global flag before the subcommand, as yoke a089a6db has
it. `grep -rn no-check-for-updates builds/kopia` returns nothing. The rendered
gate was then run end-to-end against that binary on darwin: the server became
ready and no sandbox outbound deny was logged.

## Gates

- `builds/check.sh` exits 0: 11 egress-verdict cases, 19 toolchain-gate cases,
  10 fork trees reproduce from their declarations, shellcheck and actionlint
  clean.
- `govulncheck` 0 reachable for gitea, zot and kopia (above).
- The `GO_SECURITY_OVERRIDES` go.mod diff gate passes for all three, replicated
  locally against the real pinned trees.
- kopia's rendered `no-egress-test.sh` passes against a binary built from the
  overridden tree.

## Follow-ups (not this story)

- `GO_SUPPORTED_MINORS` and `GO_AUDIT_TOOLCHAIN` need a bump each time Go
  releases a minor, which is also when a pin on the newly retired minor needs
  an EOL-minor override.
- The manager owns pushing `qiangli/*` and tagging.
