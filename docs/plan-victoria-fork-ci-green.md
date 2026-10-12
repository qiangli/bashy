# Victoria fork CI: one shared egress verdict, and pins that clear govulncheck

Sprint #434, story #1897 (`eb6f5cd6cb46`). The first CI run of all three
Victoria verified-build forks failed the same two ways:

| run | fork | pin job | `govulncheck` | Linux no-egress |
|---|---|---|---|---|
| 38311645575 | `qiangli/victoria-metrics` | OK | FAIL | FAIL |
| 38311666036 | `qiangli/victoria-logs` | OK | FAIL | FAIL |
| 38311680149 | `qiangli/victoria-traces` | OK | FAIL | FAIL |

Two independent defects, two fixes, one slice. Conductor decisions are binding;
this doc records what they were and what was measured.

## 1. no-egress: AF_UNIX is local IPC, not egress

The Linux leg traces the server with
`strace -f -e trace=connect,sendto,sendmsg,sendmmsg` inside a fresh network
namespace and judges every recorded address. The judgement was wrong for one
family: a `connect()` to the **glibc nscd / systemd-resolved socket** is an
`AF_UNIX` path on the local filesystem, not a packet leaving the host, and the
namespace guarantees it cannot become one. It was being counted as egress, so
the gate failed on a resolver handshake that went nowhere.

That verdict was already corrected for `tuwunel` in bashy `fb22eaf9` — and
then existed in **seven** near-identical inline copies (tuwunel, the three
Victoria components, gitea, kopia, zot), six of them still wrong.

**Fix:** factor the verdict into ONE copy, `builds/_shared/egress-verdict.sh`,
and ship it into each fork's `bashy-build/` next to the harness:

- `tool.env` gains `SHARED_FILES="egress-verdict.sh"`. `render.sh` copies each
  declared name from `_shared/` into `bashy-build/`, `chmod +x` for `*.sh`, and
  refuses a name that is not in `_shared/`, is not a plain file name, or is
  shadowed by the tool's own `files/`.
- Each `no-egress-test.sh` sources it and calls
  `egress_verdict "$log" PORT...` — tuwunel with its own port, the six
  collector-adjacent tools with their port plus `31415`. The allowed-port list
  is the only thing that varied, so one function reproduces every caller's
  behaviour, including tuwunel's single-port message.
- The verdict: AF_INET/AF_INET6 judged (loopback on an allowed port passes,
  everything else — including DNS and attempts the namespace makes fail — is
  egress); AF_UNIX skipped as local IPC; AF_UNSPEC/AF_NETLINK have no address
  to judge. A lookup that *does* leave the host still shows up as the
  AF_INET/AF_INET6 send to port 53 that follows, which the verdict catches.

`openclaw` keeps its own verdict: a POSIX-sh grep over a `-e trace=%network`
log in a different harness shape. Its `fork/` is byte-identical after this
change.

## 2. govulncheck: bump the pins; override the toolchain where the tag can't

The fork CI reported **12 reachable** findings across the three tools: 8 in the
stdlib plus `golang.org/x/text`, `golang.org/x/net`, `google.golang.org/grpc`
and `go.opentelemetry.io/otel`. No waivers: `vuln-ignore` is for unreachable
findings only, and these were reachable from called code.

**Pins bumped to the latest stable upstream tags**, each tag commit resolved
with `git ls-remote` and each `LICENSE` read at the tag:

| tool | tag | annotated tag object | commit | LICENSE | `go.mod` |
|---|---|---|---|---|---|
| victoria-metrics | `v1.154.0` | `e282add54bf37b36d523ea32f35b7d6e064fb8c9` | `29c43cd2fee05409499a4d1b38e950e1b7f13bd4` | Apache-2.0 | `go 1.27.2` |
| victoria-logs | `v1.53.0` | `cccc0afb52cb5799c27800ff00e1807be2d8fe1e` | `915d91904bf7f5be66ab00f88947ae7fc1431acc` | Apache-2.0 | `go 1.27.1` |
| victoria-traces | `v0.12.0` | `02f683ca9405531890b639633c67d4dba3f74223` | `fdfa32fdc58188463a9b342741bf10a748e455ce` | Apache-2.0 | `go 1.26.6` |

All three upstream GitHub releases are non-draft and non-pre-release, as the
`pin` job requires.

**Build-flag / ldflags diff reviewed, and unchanged at the new tags**: the root
`Makefile`'s `GO_BUILDINFO` is still the single
`-X …/lib/buildinfo.Version=…` stamp (so `GO_LDFLAGS` stands), the Victoria
components still take no extra build tags, `deployment/docker/Makefile` still
sets `CGO_ENABLED=1` for linux amd64/arm64 and `0` elsewhere (so `GO_CGO`
stands), `GO_PACKAGE` paths still exist, and `-pushmetrics.url` still defaults
to empty. `GO_TOOLCHAIN` was moved to each tag's `go.mod` value, which also
matches upstream's own `GO_BUILDER_IMAGE` (`golang:1.27.2` / `1.27.1` /
`1.26.6`).

### The module findings are gone; the stdlib ones need a later patch

victoria-metrics `v1.154.0` carries `x/net v0.60.0`, `x/text v0.42.0`,
`grpc v1.84.0` and `otel v1.47.0` — at or past every fixed version. The logs
and traces module graphs never contained `x/net`, `grpc` or `otel` at all.

The stdlib batch of 2026-10-08 (`GO-2026-6603`, `-6605`, `-6607`, `-6608`,
`-6609`, `-6610`, `-6611`, `-6612`, `-6613`, `-6617`) is fixed in **go1.26.9
and go1.27.2** — later than what two of the three tags name. Reachable
vulnerabilities are never waived, and `GO_TOOLCHAIN` must keep naming what
upstream builds with, so the `go` kind gained a declared, gated
`GO_TOOLCHAIN_SECURITY_OVERRIDE`:

| tool | tag's `go.mod` | built with | override |
|---|---|---|---|
| victoria-metrics | `1.27.2` | `1.27.2` | none needed |
| victoria-logs | `1.27.1` | `1.27.2` | declared |
| victoria-traces | `1.26.6` | `1.26.9` | declared |

The gate (now in **both** `kinds/go/setup.yml` and `kinds/go/audit.yml`, so the
binary and the report agree) still requires `GO_TOOLCHAIN == go.mod`, then
accepts exactly the declared override: `MAJOR.MINOR.PATCH`, same Go minor as
upstream, strictly later patch. An override can therefore never become a
language-version change. It ships in `PIN` (and the `<tool>-pin.txt` release
asset), the step logs it, and each declaration's `RELEASE_GATES` names it in
the release notes.

`audit.yml` also no longer sets `GOTOOLCHAIN` itself: it inherits the gate's
`GITHUB_ENV` value, which matters because `govulncheck` must be **built** with
the same release — its type checker has to understand sources that declare a
newer language version, and the stdlib it judges must be the one that ships.

### Measured

`govulncheck v1.9.0` (the pinned version), built with each tool's effective
toolchain, source mode over `./...`, vulndb of 2026-10-12, on darwin/arm64:

| tool | at the tag's own toolchain | with the effective toolchain |
|---|---|---|
| victoria-metrics `v1.154.0` | — (`go1.27.2`, no override) | **0 reachable** (1 module-only finding, `GO-2026-5932`) |
| victoria-logs `v1.53.0` | 10 reachable at `go1.27.1` | **0 reachable** at `go1.27.2` |
| victoria-traces `v0.12.0` | 10 reachable at `go1.26.6` | **0 reachable** at `go1.26.9` |

So the override is both necessary and sufficient, and **no
`go get module@fixed` override step was needed** — the pin bump cleared every
module-level finding on its own, so none was added. The fork CI run on
`ubuntu-24.04` is the authority; this local run is evidence for the pin review,
not a substitute for it.

## Blast radius

| fork | `fork/` change |
|---|---|
| victoria-metrics, victoria-logs, victoria-traces | pin, toolchain (+ override), shared verdict, go-kind workflow |
| gitea, kopia, zot | shared verdict (fixes the same AF_UNIX miscount) + the inert go-kind workflow text |
| tuwunel | shared verdict only — `PIN` gains `SHARED_FILES`, the harness sources the library. Its workflow is untouched (`kinds/cargo` and the skeleton were not edited) |
| openclaw | **none** — byte-identical |

## Acceptance

- `builds/render.sh <tool>` for all eight tools, then `builds/check.sh` exits 0
  (regenerate-and-diff, `bash -n`, `shellcheck -S warning`, `actionlint`).
- The shared verdict's eleven cases are pinned by `_shared/egress-verdict-test.sh`,
  which `check.sh` now runs: AF_UNIX nscd and systemd-resolved pass, loopback on
  an allowed port passes (v4, v6 and v4-mapped), AF_NETLINK and AF_UNSPEC are
  ignored, and a loopback port that is not allowed, a DNS send to `8.8.8.8` and
  a v6 egress all fail. The AF_UNIX case is a reviewed decision a well-meaning
  edit could quietly reverse, so it is now a test rather than a comment.
- The toolchain gate was driven against synthetic declarations: each of the
  three Victoria shapes resolves to the right effective toolchain, a PIN with
  no override key is inert, and the gate refuses a minor bump, the same patch,
  an earlier patch, a two-component override, junk, and a `GO_TOOLCHAIN` that
  disagrees with `go.mod`.

## Follow-up (not this slice)

- The manager pushes `fork/` to the `qiangli/*` branches and cuts
  `bashy-<tag>-1`; nothing here pushes or tags.
- **yoke's pin flip** to the new releases (`external/otel/stack/execstore.go`
  `victoriaMetricsVersion` / `victoriaLogsVersion` / `victoriaTracesVersion`)
  is its own story, after the fork CI is green. yoke is deliberately untouched.
- `openclaw`'s harness has the same latent AF_UNIX miscount in its grep-shaped
  verdict. It was not touched here (different shell, different trace flags, and
  its fork is mid-release); worth a slice of its own.
