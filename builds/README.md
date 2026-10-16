# builds/ — bashy verified builds (fork-build-sign pipeline)

bashy runs a third-party tool only from a **bashy-CI-built,
Sigstore-keyless-signed build of a pinned, permissive upstream release tag**,
with no third-party tracking and OTel pointed only at bashy's local collector
`127.0.0.1:31415` (umbrella `docs/bashy-service-supply-chain-policy.md`,
rules 1 and 2).

Every tool uses the same pipeline. A tool is **one declaration** (`tool.env`)
plus only its genuinely tool-specific files. `render.sh` generates the tree
that is installed as the orphan `bashy-build` branch of
`github.com/qiangli/<tool>`. `check.sh` proves the committed trees still
match their declarations.

```
builds/
  render.sh                 render.sh TOOL [OUT]  -> builds/TOOL/fork (generated)
  check.sh                  regenerate-and-diff gate (+ the rendered job set and
                            pin-gate count, bash -n, shellcheck, actionlint,
                            the shared verdict's self-test)
  _shared/
    bashy-build.yml         workflow skeleton: pin -> build matrix -> [audit] -> release
    verify-pin.sh           provenance gate, shipped into every fork
    egress-verdict.sh       THE strace verdict, shipped into every fork that
                            declares SHARED_FILES (sourced by no-egress-test.sh)
    egress-verdict-test.sh  its self-test; check.sh runs it (not shipped)
    kinds/cargo/            setup.yml / build.yml / audit.yml steps for Rust tools
    kinds/go/               the same three for Go tools
  <tool>/
    tool.env                THE declaration (shipped verbatim as bashy-build/PIN)
    files/                  tool-specific files, copied into bashy-build/
    setup.yml build.yml audit.yml   optional step fragments; each overrides the kind's
    README.md               knob tables, upstream evidence, binmgr pin
    fork/                   GENERATED — never edit by hand
```

## Tools

| tool | upstream | pinned tag | license at the tag | kind | who runs it |
|---|---|---|---|---|---|
| `ollama` | `ollama/ollama` | `v0.21.2` | MIT root; full input review pending | `custom` | [runner recipe, release blocked](ollama/README.md) |
| `openclaw` | `openclaw/openclaw` | `v2026.10.2` | see its README | `custom` | the OpenClaw gateway |
| `tuwunel` | `matrix-construct/tuwunel` | `v1.9.4` | Apache-2.0 | `cargo` | the Matrix homeserver |
| `victoria-logs` | `VictoriaMetrics/VictoriaLogs` | `v1.53.0` | Apache-2.0 | `go` | yoke `external/otel/stack` |
| `victoria-metrics` | `VictoriaMetrics/VictoriaMetrics` | `v1.154.0` | Apache-2.0 | `go` | yoke `external/otel/stack` |
| `victoria-traces` | `VictoriaMetrics/VictoriaTraces` | `v0.12.0` | Apache-2.0 | `go` | yoke `external/otel/stack` |
| `gitea` | `go-gitea/gitea` | `v28.1.0` | MIT | `go` (own `build.yml`) | yoke `external/loom` |
| `kopia` | `kopia/kopia` | `v0.23.1` | Apache-2.0 | `go` | yoke `external/kopia` |
| `zot` | `project-zot/zot` | `v2.1.22` | Apache-2.0 | `go` | yoke `external/zot` |
| `seaweedfs` | `seaweedfs/seaweedfs` | `4.48` | Apache-2.0 | `go` | yoke `external/seaweedfs` |
| `act_runner` | `gitea/runner` (**gitea.com**) | `v5.0.0` | MIT | `go` | yoke `external/actrunner` |
| `k3s` | `k3s-io/k3s` | `v1.36.1+k3s1` | Apache-2.0 | `go` (own `build.yml`, `audit.yml`) | the outpost runtime image ([audit blocked at this pin](k3s/README.md#release-blocked-at-this-pin)) |
| `cni-plugins` | `containernetworking/plugins` | `v1.5.1` | Apache-2.0 | `go` (own `build.yml`, `audit.yml`) | the outpost runtime image |
| `metrics-server` | `kubernetes-sigs/metrics-server` | `v0.9.0` | Apache-2.0 | `go` | the outpost runtime image |
| `runc` | `opencontainers/runc` | `v1.5.2` | Apache-2.0 | `go` (own `build.yml`) | the outpost runtime image |
| `coredns` | `coredns/coredns` | `v1.14.7` | Apache-2.0 | `go` | the outpost runtime image (k3s cluster DNS) |
| `pause` | `kubernetes/kubernetes` (`build/pause`) | `v1.37.1` | Apache-2.0 | `go` (own `build.yml`, `audit.yml`; ships a pure-Go port of `pause.c`) | the outpost runtime image (k3s `--pause-image`) |
| `tailscale` | `tailscale/tailscale` | `v1.104.1` | BSD-3-Clause | `go` (own `build.yml`, `audit.yml`) | the outpost runtime image |

Each pin is the release yoke runs today, read from `external/<tool>` or
`pkg/fleet/baseline/services/<tool>.yaml` — except the three Victoria
components, whose pins **lead** yoke (`external/otel/stack/execstore.go` still
names `v1.147.0` / `v1.51.0` / `v0.9.4`). They were bumped to the latest stable
upstream tags to clear every REACHABLE `govulncheck` finding on the fork CI;
yoke's flip to the signed fork releases follows once that CI is green and the
bashy tags are cut. `seaweedfs` and `act_runner` are the same shape: yoke
resolves seaweedfs with `DefaultVersion = "latest"` and act_runner with a
`binmgr.URLSpec` against `dl.gitea.com`, so both pins name the latest stable
upstream release rather than something yoke already states, and each tool's
README §Operator steps says what yoke has to change. Every license above was
checked at the pinned tag, not at HEAD.

### Two upstreams that are not GitHub-and-vX.Y.Z

Both were blocked on the pipeline rather than on licensing (`seaweedfs` is
Apache-2.0, `gitea/act_runner` MIT). Each needed a `_shared/` change, and both
now have one. They stay called out here because every *other* tool is the easy
shape, and these two are where to look when a third odd upstream arrives.

- **`seaweedfs` tags bare `X.Y`** (`4.48`; the last `v*` tag it ever cut is
  `v3.33`). Solved by §The declared release-tag form below. Its bashy tag is
  `bashy-4.48-N` and the workflow trigger is `bashy-[0-9]*`.
- **`act_runner` has no GitHub upstream.** Its home is `gitea.com` and its
  binaries are published on `dl.gitea.com`, which is why yoke resolves it with
  a `binmgr.URLSpec` rather than a `GitHubSpec`. Solved by §Non-GitHub
  upstreams below: the build clones the tag with `git`, and the `pin` job
  queries the forge's own release API. Upstream has since renamed the
  repository to `gitea/runner`; `builds/act_runner/README.md` §The upstream
  rename says why the bashy tool keeps the old name.

### Windows legs

Prima and Ollama already ship signed Windows legs on `windows-2025` using
Git Bash and the Windows Defender Firewall no-egress harness. `KIND=go`
also supports native Windows builds: declare `WINDOWS_SHELL=bash`, use the
full `<name>-windows-amd64.exe` asset name in `MATRIX`, `GO_CGO`,
`RELEASE_ASSETS` and `RELEASE_EXECUTABLES`, and select `windows-2025`.
Gitea (loom), SeaweedFS, zot, VictoriaLogs, VictoriaMetrics and VictoriaTraces
declare these legs with `CGO_ENABLED=0`.
Gitea runs its Node/pnpm frontend and bindata generation on Windows too.

The Go service gates reuse Ollama's prima-derived Defender harness, now in
`_shared/windows-defender-egress.ps1`. A service exercise starts the pinned
binary with its shipped no-tracking configuration, probes its loopback HTTP
surfaces and observes it for 30 seconds. Disposable outbound block rules
cover both the executable and the exercise interpreter. The gate requires a
blocked negative control (access-denied or a firewall DROP), successful
exercise evidence, no observed non-loopback connection, and policy cleanup.
It runs only on GitHub-hosted Windows runners. Linux still uses the shared
`egress-verdict.sh` strace verdict; macOS uses sandbox-exec. The Windows
firewall permits loopback generally, unlike the Unix gates' port allowlists.

A successful cross-build is compile evidence, not a Windows runtime gate.
Every Windows leg must pass its native smoke and no-egress exercise before
signing; tools without a Windows leg use a Linux venue on Windows hosts.

## What the shared pipeline does (every tool)

- **pin**: refuses to run outside `qiangli/<tool>`, refuses an upstream tag
  that is not the declared `UPSTREAM_TAG_FORM` (`vX.Y.Z` by default), refuses a
  bashy tag other than `bashy-<upstream tag>-N`, and refuses an upstream
  release that is a draft or pre-release (queried at `UPSTREAM_API_BASE`, which
  is GitHub's API unless the upstream lives elsewhere).
- **build** (one leg per `MATRIX` entry): checks out the fork (`fork/`) and the
  upstream tag (`src/` — `actions/checkout` for a GitHub upstream, a pinned
  `git clone` otherwise); `verify-pin.sh` fails unless HEAD and the tag both
  equal `UPSTREAM_COMMIT`, the tree is pristine and (if declared)
  `RUST_TOOLCHAIN` matches; installs `APT_PACKAGES` on Linux; runs the
  `setup` steps (which for the `go` kind apply `GO_SECURITY_OVERRIDES`), then
  each `PRE_BUILD_GATES` script as `bashy-build/<gate> src`,
  then the `build` steps (which write their outputs to `out/`); then
  `bashy-build/no-egress-test.sh "$EGRESS_TARGET"`, then uploads `out/*`.
  A tool may also declare an `egress-evidence.yml` step fragment (rendered
  from `@@INCLUDE egress-evidence@@` right after the gate): it uploads the
  gate evidence file with `if: always()` so a gate failure still leaves its
  cause in an artifact. Tools without the fragment render byte-identically.
- **audit** (only when an `audit` fragment exists): the same checkout + pin
  verification, then the same `setup` steps as the build job — the `go` kind's
  audit fragment `@@INCLUDE setup@@`s it rather than keeping a second copy, so
  the overridden tree cannot drift between the two jobs — then the `audit`
  steps (writing SBOM and other outputs to `out/`).
- **release**: copies `RELEASE_EXTRA` files from `bashy-build/`, chmods
  `RELEASE_EXECUTABLES`, fails if any `RELEASE_ASSETS` file is missing, writes
  `SHA256SUMS`, then `cosign sign-blob --bundle` signs every asset (cosign
  `COSIGN_VERSION`) and self-verifies it against the exact identity. It then
  runs `actions/attest` over `SHA256SUMS` and publishes a GitHub release on the
  fork.

Sigstore identity of a release cut from tag `T`:
`https://github.com/qiangli/<tool>/.github/workflows/bashy-build.yml@refs/tags/T`,
issuer `https://token.actions.githubusercontent.com`. Every action is pinned by
commit sha in the skeleton. Bump an action there once, and every tool gets it.

## The declaration — `builds/<tool>/tool.env`

Shell-sourceable `KEY=VALUE` only. It is shipped verbatim as
`bashy-build/PIN`, so workflow steps and scripts `. bashy-build/PIN` to read it.

| key | meaning |
|---|---|
| `TOOL` | directory name; fork is `FORK_REPO=qiangli/$TOOL` |
| `UPSTREAM_REPO`, `UPSTREAM_SLUG` | upstream URL and `owner/repo`. The URL's host decides the forge (see §Non-GitHub upstreams); `UPSTREAM_REPO` must end in `UPSTREAM_SLUG` |
| `UPSTREAM_TAG`, `UPSTREAM_COMMIT` | pinned release tag and its full 40-hex commit |
| `UPSTREAM_TAG_FORM` | optional; the shape of upstream's release tags — `vX.Y.Z` (default), `X.Y` or `vX.Y.Z+k3sN`. See §The declared release-tag form |
| `UPSTREAM_API_BASE` | optional for a GitHub upstream (defaults to `https://api.github.com`), **required** otherwise: the forge's REST root, which must answer `/repos/SLUG/releases/tags/TAG` with `.draft` and `.prerelease` |
| `KIND` | step-fragment kind under `_shared/kinds/` (`cargo`, `go`), or `custom` (the tool supplies its own fragments) |
| `COSIGN_VERSION` | cosign release used to sign (`v3.1.3`) |
| `MATRIX` | `ASSET=RUNNER ...`; each leg's artifact is named `ASSET` |
| `APT_PACKAGES` | Linux host packages (include the no-egress tools: `strace iproute2 curl`, ...) |
| `PRE_BUILD_GATES` | scripts in `files/` run as `<gate> src` before the build |
| `SHARED_FILES` | plain file names under `_shared/`, copied into `bashy-build/` next to the tool's own `files/`; `*.sh` is made executable. One reviewed copy serves every tool that declares it (`egress-verdict.sh` today). A name that also exists in the tool's `files/` fails the render — no silent shadowing |
| `EGRESS_TARGET` | argument of `no-egress-test.sh` (may be a `${{ }}` expression) |
| `RELEASE_EXTRA` | `SRC=ASSET` copies from `bashy-build/` into the release |
| `RELEASE_ASSETS` | files that must exist before signing |
| `RELEASE_EXECUTABLES` | assets to `chmod +x` |
| `RELEASE_NOTE`, `RELEASE_GATES` | release-notes lines |
| kind keys | `cargo`: `RUST_TOOLCHAIN CARGO_AUDIT_VERSION CARGO_CYCLONEDX_VERSION CARGO_PACKAGE CARGO_BIN CARGO_SBOM_MANIFEST`, optional `files/features.env` (`FEATURES` -> `--no-default-features --features`), required `files/audit-ignore` |
| kind keys | `go`: `GO_TOOLCHAIN GO_PACKAGE GO_LDFLAGS GO_BUILD_TAGS GO_BUILD_FLAGS GO_BUILD_ENV GO_CGO GO_VERSION_ARG GOVULNCHECK_VERSION GO_SBOM_VERSION`, `GO_AUDIT_TAGS` (required once `GO_BUILD_TAGS` is non-empty), optional `GO_TOOLCHAIN_SECURITY_OVERRIDE`, `GO_SECURITY_OVERRIDES`, `GO_AUDIT_TOOLCHAIN` and `GO_SUPPORTED_MINORS`, required `files/vuln-ignore` |

Templates use `@@KEY@@`. A key that is not declared fails the render. That
includes keys a tool's own fragments use, such as openclaw's `NODE_VERSION` and
`PNPM_VERSION`. A few template values are **derived** by `render.sh` rather
than declared — `UPSTREAM_FORGE`, `UPSTREAM_TAG_REGEX`, `BASHY_TAG_GLOB` — so
that the one place a form or a forge is interpreted is `render.sh`, and the
gates it renders into the workflow and into `verify-pin.sh` cannot drift apart.
Besides `@@KEY@@`, templates support `@@MATRIX@@`, `@@INCLUDE name@@`,
`#@@IF name@@` (is that fragment present?) and `#@@IFEQ VAR value@@`, each with
an optional `#@@ELSE@@` and a closing `#@@END@@`. **Conditionals nest**, and a
false outer branch wins: `render.sh` keeps one digit per open conditional and
emits a line only when every one of them does. Getting that wrong is not
something `check.sh`'s diff can catch — a suppressed job's steps leak into its
predecessor, and the result is still deterministic and still valid YAML — which
is why `check.sh` also asserts the rendered **job set** (`pin build release`,
plus `audit` iff the tool has an audit fragment) and that the pin gate appears
exactly once per job that checks out `src`.

### The declared release-tag form

Most upstreams tag `vMAJOR.MINOR.PATCH`, and that is the default. Some do not:
seaweedfs ships bare `MAJOR.MINOR` (`4.48`), and the last `v`-prefixed tag it
ever cut is `v3.33`. So the shape is **declared**, never sniffed from the pin —
a typo in a bumped tag is still refused:

```
UPSTREAM_TAG=4.48
UPSTREAM_TAG_FORM=X.Y     # omitted = vX.Y.Z
```

`render.sh` maps the form to its regex in exactly **one** `case`, and renders
that regex into both gates that need it — the workflow's `pin` job and the
shipped `bashy-build/verify-pin.sh` (which is rendered rather than copied for
this reason) — so the two cannot disagree. It also derives the workflow's own
trigger glob: `bashy-v*` for `vX.Y.Z`, `bashy-[0-9]*` for `X.Y`. An
undeclared form fails the render.

The **bashy** tag needs no form of its own. Both gates match
`^bashy-(.+)-([1-9][0-9]*)$` and compare the captured middle against
`UPSTREAM_TAG`, which has already been held to the declared shape — so
`bashy-4.48-1` and `bashy-v2.1.21-3` are both exactly `bashy-<the pinned
upstream tag>-N` with no second copy of the form anywhere.

k3s tags `vX.Y.Z+k3sN` (`v1.36.1+k3s1`): the Kubernetes release plus k3s's
build counter. Its form keeps the `bashy-v*` trigger; its bashy tag is
`bashy-v1.36.1+k3s1-N`.

Adding a form means one more `case` arm (its regex and its trigger glob).
Keep arms narrow: the point is to refuse an `-rc1`, a `-dev` or a typo, not to
accept whatever upstream feels like tagging.

### Non-GitHub upstreams

The fork is always on GitHub, but the **upstream** need not be: act_runner's
home is `gitea.com`. `render.sh` **derives** the forge from `UPSTREAM_REPO`
rather than taking a second declaration, so the URL and the forge cannot
disagree. A non-`github.com` host changes exactly two things:

- the upstream checkout becomes
  `git clone --depth 1 --single-branch --branch <tag> "$UPSTREAM_REPO" src`
  instead of `actions/checkout` with `repository:` (the
  `#@@IFEQ UPSTREAM_FORGE github@@` block in the skeleton, in both the `build`
  and the `audit` job);
- the `pin` job queries `UPSTREAM_API_BASE` instead of
  `https://api.github.com`. That key is required off GitHub, since no default
  is possible and the gate cannot check a release it cannot query. Gitea serves
  the identical `/repos/SLUG/releases/tags/TAG` shape with the same `.draft`
  and `.prerelease` fields, so the draft/pre-release gate is unchanged.

**The pin gate is identical either way.** `verify-pin.sh` still demands that
`src/`'s HEAD *and* `refs/tags/<tag>` both resolve to `UPSTREAM_COMMIT` over a
pristine tree; a shallow single-branch clone carries the tag ref, so a second
forge buys no weaker provenance than `actions/checkout` does.

What a second forge does NOT get is GitHub's attestation of the *upstream*
release — but the pipeline never relied on that: provenance here is the pinned
commit, and the signature is bashy's own over the artifact.

### Recorded security overrides (the `go` kind)

#### `GO_TOOLCHAIN_SECURITY_OVERRIDE` — the toolchain half

`GO_TOOLCHAIN` must equal the `go`/`toolchain` directive in the pinned tag's
`go.mod` — the kind's setup step gates that, so a pin bump cannot silently
build on whatever the runner ships. That leaves one real gap: an upstream tag
whose own toolchain **predates a Go stdlib security fix**, with no later
upstream tag yet. Waiving the finding is not an option (reachable
vulnerabilities are never waived; `vuln-ignore` is for unreachable ones), and
bumping `GO_TOOLCHAIN` would be a lie about what upstream builds with.

So the declaration may name a *recorded security override*:

```
GO_TOOLCHAIN=1.27.1                     # what the tag's go.mod says
GO_TOOLCHAIN_SECURITY_OVERRIDE=1.27.2   # what bashy builds with, and why (comment)
```

Both the `build` and `audit` jobs then run `GOTOOLCHAIN=go<override>`, so the
shipped binary and the vulnerability report agree. It is shipped in `PIN` (and
so in the `<tool>-pin.txt` release asset), the step logs it, and the
declaration's `RELEASE_GATES` names it in the release notes. The override
retires itself the moment an upstream tag carries the fix.

The gate accepts **exactly two shapes** and nothing else. The value must be
`MAJOR.MINOR.PATCH` and strictly later than `GO_TOOLCHAIN` either way — a
downgrade and a no-op are always refused.

**In-minor patch bump** — the ordinary case, and the only one allowed while the
tag's minor is still supported. A later patch of the tag's own minor, so the
language version never moves. Choose the newest patch of that minor.
`builds/gitea` (`1.27.1` -> `1.27.2`) is the worked example.

**EOL-minor security override** — Go ships security fixes for its two newest
minors only, so a minor goes end-of-life the moment the next one is released
(go1.27 retired 1.25). A pin on a retired minor has **no later patch to move
to**, and staying put cannot be made safe at any patch level. Only then may the
override leave the upstream minor, and only to the **oldest supported minor's
latest patch** — the smallest language-version jump that gets fixes again:

```
GO_TOOLCHAIN=1.25.11                    # a minor that no longer gets fixes
GO_TOOLCHAIN_SECURITY_OVERRIDE=1.26.9   # EOL-minor security override
```

`builds/kopia` is the worked example. A jump while the tag's minor **is** still
supported is refused (there a later patch exists, and the patch bump is the
whole of what review sanctioned), as is a jump to anything but the oldest
supported minor. The supported set is `GO_SUPPORTED_MINORS` in
`builds/render.sh` — oldest first, one place — and keeping it current is part
of reacting to a Go release.

#### `GO_SECURITY_OVERRIDES` — the module half

The same gap exists one level down: an upstream tag can require a
**dependency** that predates a security fix, with no later upstream tag
carrying the bump. Waiving is still not an option, and editing the pinned
sources is not either, so the declaration may name the exact modules to raise:

```
GO_SECURITY_OVERRIDES="golang.org/x/net@v0.60.0"   # space-separated; empty = no-op
```

Right after the toolchain step, the `go` kind runs `go get <spec>` in `src/`
for each entry. **Empty or absent is a no-op**, which is what every tool that
needs no override gets.

What makes this safe is the gate, not `go get`: afterwards `go.mod` may differ
**only in the require lines of exactly the declared modules**. Anything else
fails the build — a new or removed require, a `replace`/`exclude`/`retract`
edit, or an undeclared module moved as a transitive effect (if one is genuinely
needed, declare it too, so it is reviewed). The comparison is over `go mod edit
-json`, not the file text, so a reformat or a moved block cannot hide a real
change. `go.sum` is deliberately **not** gated: `go get` has to add the new
hashes. There is no `go mod tidy` — tidy rewrites the whole graph, which this
gate could not tell apart from a supply-chain substitution.

Taking several fixes into an old graph can move a lot of require lines, and
each one has to be named. The list is then **derived, not hand-picked**: run
`go get` for the fixes you need, read off every module MVS moved, and declare
each at the version MVS selected. Pinning the selected version is what keeps
the extra entries no-ops — naming a module at a version *below* what MVS picked
would silently downgrade it. Regenerate the whole list when the pin moves
rather than patching it. `builds/kopia` is the worked example: five fixes, 23
declared modules.

**The `go` and `toolchain` directives have their own gate.** A dependency
carrying a security fix can require a newer *language* version — `x/net`
v0.60.0 and `x/crypto` v0.56.0 both declare `go 1.26.0` — so taking the fix
into an older module forces `go get` to raise `go.mod`'s `go` line. The fix is
unreachable otherwise, so the directive may rise, but **never past the
toolchain this build actually runs**, which the step above has already gated.
That cap is what keeps the allowance honest: under an in-minor patch override
it confines the language version to the tag's own minor, and under an
EOL-minor override it permits exactly the move that override already
sanctioned. A directive that drops, or one above the effective toolchain, is
refused. The `toolchain` line may only be dropped or lowered — `GOTOOLCHAIN` in
the environment is what selects the release.

The step runs in **both** the `build` and the `audit` job, after the same
`verify-pin.sh`, so the same overridden tree is what govulncheck judges and
what the shipped binary is compiled from — and the SBOM records the version
that actually shipped. It is shipped in `PIN` (and so in the `<tool>-pin.txt`
release asset), the step logs the resulting diff, and the declaration's
`RELEASE_GATES` names it in the release notes.

`builds/seaweedfs` is the worked example and needs both halves: 12 reachable
stdlib findings fixed by `GO_TOOLCHAIN_SECURITY_OVERRIDE=1.26.9`, then 5
remaining `golang.org/x/net` findings fixed by
`GO_SECURITY_OVERRIDES="golang.org/x/net@v0.60.0"`, for zero reachable
findings and an empty `vuln-ignore`.

### What the audit judges (the `go` kind)

`govulncheck` runs in source mode over **the package the release builds**
(`GO_PACKAGE`), with the build tags the release is built with (`GO_AUDIT_TAGS`)
and the same `GO_BUILD_ENV`. "Reachable" then means reachable *in the signed
artifact*, which is the only question a release gate can honestly answer. The
CycloneDX SBOM is generated with the same tags (through `GOFLAGS`), because
tags change Go's module selection — otherwise the SBOM of a reduced build would
list dependencies the binary does not contain.

`./...` cannot answer that question. It makes every package in the repository a
root: test helpers, and the extension packages a reduced tag set deliberately
keeps out of the binary. Build tags do not save it — zot's
`pkg/extensions/search/cve/trivy` carries no build constraint at all, so
`./...` calls trivy's unfixed `GO-2026-4919` reachable in a binary that links
none of it. The only ways out would be a waiver (never, for a reachable
finding) or refusing to ship a tool that is not actually vulnerable.

`GO_AUDIT_TAGS` must therefore be the tags the release is **built** with, and
`render.sh` requires it as soon as `GO_BUILD_TAGS` is non-empty, even to say
"none" — the audit configuration is never left to a default. The one sanctioned
divergence is a tag that needs generated sources the audit job does not produce:
`builds/gitea` builds with `bindata` and audits without it, because `bindata`
selects the generated `modules/public/bindata.go` over the `!bindata`
filesystem fallback, which changes which file serves an embedded asset, not
which modules the binary requires.

### The audit tools have their own toolchain

`govulncheck` and `cyclonedx-gomod` are installed with `GO_AUDIT_TOOLCHAIN`
(`builds/render.sh`, go1.27.2 today), **independent** of the toolchain that
builds the artifact. They are Go modules with `go` lines of their own:
`govulncheck` v1.9.0 declares `go 1.26.0`, so `go install` under an older
pinned toolchain fails outright, before any analysis happens — which is how
`qiangli/kopia` run 38326642419 died on kopia v0.23.1's go1.25.11. Coupling the
two would hold every tool's audit hostage to its upstream's toolchain age.

This gives the audit nothing it should not have. `go install pkg@version`
resolves in module-agnostic mode and never reads `src/go.mod`, so
`GO_AUDIT_TOOLCHAIN` cannot reach the module being judged; and the **analysis**
still runs at the module's own effective toolchain, because `govulncheck` shells
out to `go` and `GOTOOLCHAIN` in the environment pins that to the release the
artifact is built with. So the stdlib `govulncheck` judges and the language
version it type-checks are the artifact's, not the audit tool's. Keep
`GO_AUDIT_TOOLCHAIN` at the newest patch of the newest supported minor.

## Conventions every tool must follow (render.sh enforces the first two)

1. `files/otel.env` sets `OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:31415`,
   and every `*ENDPOINT=` in it is loopback. If the tool has no OTel SDK,
   disable it there as well (`OTEL_SDK_DISABLED=true`, exporters `none`).
2. `files/no-egress-test.sh TARGET` is the runtime gate. It must start the
   built artifact in an isolated network (Linux: `unshare` netns + `strace`),
   exercise it over loopback, and **fail** on any non-loopback connection
   attempt. The **harness** is tool-specific — a server needs a readiness probe
   and a CLI needs representative invocations — but the **verdict** is not:
   declare `SHARED_FILES="egress-verdict.sh"`, source it, and call
   `egress_verdict "$log" PORT...`. Copy the closest existing harness: tuwunel
   for a native binary, or openclaw for a Node app with an OTLP mock.

   The verdict is one reviewed decision, so a judgement call lands once:
   **AF_UNIX is local IPC, never egress.** A connect to the glibc nscd or
   systemd-resolved socket is a file on this host, not a packet leaving it;
   counting it as egress is what failed the Linux leg on tuwunel and then on
   all three Victoria forks. A lookup that does leave still appears as the
   AF_INET/AF_INET6 send to port 53 that follows, which the verdict catches.
   openclaw's harness keeps its own POSIX-sh grep verdict (different trace
   flags, different shell).
3. No-tracking knobs go in `files/no-tracking.env` and/or a shipped config
   file. Document each knob in the tool README with its upstream source line.
4. Audit waivers live in a reviewed file in `files/` with one justification
   per entry, such as `audit-ignore` (cargo) or `audit-exceptions` (openclaw).
   Empty means nothing is waived.

## Adding a tool

1. `mkdir builds/<tool>/files`. Write `tool.env` and copy the closest one:
   `tuwunel` for Rust/cargo, or `openclaw` for a custom toolchain.
2. Add `files/otel.env`, `files/no-egress-test.sh` (executable), any
   no-tracking config and the audit-waiver file.
3. If `_shared/kinds/<KIND>` exists, you are done with steps. If it does not,
   write `setup.yml` / `build.yml` / (optional) `audit.yml` next to `tool.env`.
   These are workflow steps at 6-space indent. The build steps must leave the
   release outputs in `out/`. For a toolchain that a second tool will reuse,
   move the fragments to `_shared/kinds/<kind>/` instead. A tool may also
   override ONE fragment of an existing kind and inherit the rest: `gitea` is
   `KIND=go` with its own `build.yml`, because the `bindata` tag embeds a
   vite-built frontend that the generic Go build knows nothing about.
4. `builds/render.sh <tool> && builds/check.sh`. Commit `tool.env`, `files/`,
   the fragments and the generated `fork/`.
5. Operator (not in bashy CI): in a clone of `qiangli/<tool>`, run
   `git switch --orphan bashy-build && cp -R <bashy>/builds/<tool>/fork/. .`,
   commit, push, and tag `bashy-<upstream tag>-1`. Then pin the release in
   yoke binmgr with the identity above.

Bumping a pin means editing `tool.env`, re-running `render.sh`, reviewing the
`fork/` diff, and cutting `bashy-<new tag>-1`. Edit `_shared/` for the
pipeline itself. `check.sh` then shows every fork that has to be re-synced.
