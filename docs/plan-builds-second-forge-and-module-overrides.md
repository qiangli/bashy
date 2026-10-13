# Plan — a second forge, bare release tags, and the module half of the security override

Sprint 434, story #1900. Closes the three `builds/_shared` gaps that
`plan-builds-go-kind-services.md` recorded as blocking `act_runner` and
`seaweedfs`, and adds both declarations.

Scope: `builds/_shared`, `builds/render.sh`, `builds/README.md`, and the two
new tool directories. Every other tool's `fork/` is re-rendered (the shared
skeleton changed), but no other tool's **declaration** is touched.

## 1. `GO_SECURITY_OVERRIDES` — the module half of a recorded override

`GO_TOOLCHAIN_SECURITY_OVERRIDE` (added for the Victoria forks) fixes an
upstream tag whose *toolchain* predates a Go stdlib security fix. The same gap
exists one level down: a tag can require a **dependency** that predates a fix,
with no later upstream tag carrying the bump. Reachable vulnerabilities are
never waived, and the pinned sources may not be edited, so the declaration
names the exact modules to raise.

```
GO_SECURITY_OVERRIDES="golang.org/x/net@v0.60.0"   # space-separated; empty = no-op
```

- A step in the `go` kind's `setup.yml` **and** `audit.yml`, immediately after
  the toolchain step (so `GOTOOLCHAIN` is already pinned) and therefore after
  `verify-pin.sh` has passed over a pristine tree. The same overridden `src/`
  feeds the audit and the build, so the govulncheck report and the SBOM
  describe the binary that actually ships.
- `go get <spec>` per entry, in `src/`. **No `go mod tidy`** — tidy rewrites
  the whole graph, which the gate below could not tell apart from a
  supply-chain substitution.
- **The gate is the mechanism.** Afterwards `go.mod` may differ only in the
  require lines of exactly the declared modules. A new/removed require, a
  changed `go`/`toolchain` directive, a `replace`/`exclude`/`retract` edit, or
  an undeclared module moved transitively all fail the build. Comparison is
  over `go mod edit -json` (not file text), so a reformat or a moved block
  cannot hide a change. `go.sum` is deliberately not gated — `go get` must add
  the new hashes.
- Shipped in `PIN` (and so in `<tool>-pin.txt`), logged by the step, named in
  `RELEASE_GATES`. Empty/absent is a no-op, so existing forks gain only the
  new wiring.
- `render.sh` additionally syntax-checks each entry and refuses the key on a
  non-`go` kind, so a typo fails at render time instead of in CI.

Rejected: a shipped `_shared/*.sh` helper. The logic has to run in both jobs,
and the toolchain step it pairs with is already inline in both fragments;
keeping them together reads better than splitting one half out.

## 2. `UPSTREAM_TAG_FORM` — bare `X.Y` release tags

seaweedfs tags bare `MAJOR.MINOR` (`4.48`); its last `v`-prefixed tag is
`v3.33`. The form is **declared**, never sniffed, so a typo is still refused.

`render.sh` maps a form to its regex in exactly one `case` and renders that
regex into both gates — the workflow's `pin` job and the shipped
`verify-pin.sh`. That is why `verify-pin.sh` is now **rendered rather than
copied**: it makes the mapping single-source instead of a third copy. The same
`case` derives the workflow's trigger glob (`bashy-v*` / `bashy-[0-9]*`).

The **bashy** tag needs no form: both gates match
`^bashy-(.+)-([1-9][0-9]*)$` and compare the captured middle against
`UPSTREAM_TAG`, which has already been held to the declared shape. So
`bashy-4.48-1` works with no second copy of the form anywhere — and this is
strictly no weaker than the old `^bashy-(v[0-9]+\.[0-9]+\.[0-9]+)-N$`, because
the upstream tag's shape is gated separately on the line above.

## 3. A non-GitHub upstream

act_runner's home is `gitea.com`. The forge is **derived** from
`UPSTREAM_REPO`'s host rather than separately declared, so the two cannot
disagree. A non-`github.com` host changes two things:

- the upstream checkout becomes
  `git clone --depth 1 --single-branch --branch <tag> "$UPSTREAM_REPO" src`
  instead of `actions/checkout` with `repository:` — a new
  `#@@IFEQ VAR value@@` render directive, used in the `build` and `audit` jobs;
- the `pin` job queries `UPSTREAM_API_BASE` (required off GitHub, defaulted to
  `https://api.github.com` otherwise). Gitea answers the identical
  `/repos/SLUG/releases/tags/TAG` shape with the same `.draft`/`.prerelease`,
  so the draft/pre-release gate is unchanged.

**The pin gate is identical.** `verify-pin.sh` still demands HEAD *and*
`refs/tags/<tag>` both equal `UPSTREAM_COMMIT` over a pristine tree; a shallow
single-branch clone carries the tag ref (verified against
`gitea.com/gitea/runner` at `v5.0.0`). `render.sh` also requires an `https://`
URL and that `UPSTREAM_REPO` end in `UPSTREAM_SLUG`.

## 3a. The renderer had to learn to nest (found while doing #3)

The forge conditional in the **audit** job sits inside the existing
`#@@IF audit@@`, and `render()` tracked conditionals with a single `skip`
flag. Directives are processed even while skipping (they have to be, to stay
balanced), so the inner `#@@END@@` reset `skip=0` and **re-opened a suppressed
outer branch**: for openclaw — the one tool with no audit fragment — the audit
job's `checkout` / `verify-pin` / `upload audit outputs` steps were appended to
the end of its *build* job.

`check.sh` passed it. That is the interesting part: the regenerate-and-diff
gate only proves the committed tree is reproducible from the declaration, and
the leak was deterministic; `actionlint` passed it too, because the result is
still well-formed YAML with well-formed steps.

Fixed two ways:

- `render()` now keeps a **stack** — one digit per open conditional — and emits
  a line only when every open conditional does, so a false outer branch wins.
  It also refuses an unbalanced template (`#@@ELSE@@`/`#@@END@@` with nothing
  open, or an unclosed conditional at EOF).
- `check.sh` gained the two **structural** assertions the diff cannot make: the
  rendered job set must be `pin build release` plus `audit` iff the tool has an
  audit fragment, and `verify HEAD == pinned commit` must appear exactly once
  per job that checks out `src`. The second is what fails on the leak above.

## 4. The two declarations

Both: `KIND=go`, 4 legs on free standard runners (`ubuntu-24.04`,
`ubuntu-24.04-arm`, `macos-15`, `macos-15-intel`),
`SHARED_FILES="egress-verdict.sh"`, `otel.env` at `127.0.0.1:31415`, empty
`vuln-ignore`.

**`builds/seaweedfs`** — `4.48` (`UPSTREAM_TAG_FORM=X.Y`) →
`530be3e37337488ecc34d58441e0bc476e121c93`, Apache-2.0, `weed` from `./weed`.
The worked example for both override halves (local govulncheck, `v1.9.0`):

| toolchain | reachable findings |
|---|---|
| `go1.26.6` (the tag's own) | **12**, all stdlib, all fixed in `go1.26.9` |
| `go1.26.9` | **5**, all `golang.org/x/net` v0.58.0, all fixed in `v0.60.0` |
| `go1.26.9` + `x/net@v0.60.0` | **0** |

Three module-level findings remain and are informational (no called-function
frame): `GO-2022-0635`/`GO-2022-0646` (`aws-sdk-go`), `GO-2026-5932`
(`x/crypto`). The `x/net` bump is a one-line `go.mod` diff, which the gate
accepts.

seaweedfs is the only tool in `builds/` whose **tracking defaults to on**:
`-master.telemetry` / `-telemetry` default `true` and post to
`https://telemetry.seaweedfs.com/api/collect`. The knob table in its README
cites every flag to its upstream line, and the no-egress harness runs the full
`weed server` stack (master/volume/filer/S3) with `-master.telemetry=false`
and `-ip=127.0.0.1`.

**`builds/act_runner`** — `v5.0.0` →
`8b14a38d4ad6a1cb40dabb953a8a65656de70301`, MIT,
`UPSTREAM_API_BASE=https://gitea.com/api/v1`. Needs only the toolchain half:
`go1.27.1` carries **10** reachable stdlib findings, all fixed in `go1.27.2`,
under which govulncheck reports **0** reachable findings anywhere — so
`GO_SECURITY_OVERRIDES=""`.

Upstream **renamed** `gitea/act_runner` → `gitea/runner` (binary
`gitea-runner`); the canonical name is pinned so nothing depends on the 301,
while the bashy tool keeps `act_runner` (yoke's binmgr name, and the fork repo).

Its one unconfigured outbound call is the find: with `cache.host` empty, the
cache server learns its own address by dialling **`8.8.8.8:80`**
(`act/common/outbound_ip.go:24`, from `act/artifactcache/handler.go:159`), on
every start. `files/config.yaml` pins `cache.host`, and the no-egress harness
fails the build if it ever comes back. `daemon`/`register` are deliberately not
the gate's subject (both are *supposed* to reach their configured instance);
the harness runs `--version`, `config generate`, `config get` and then
`cache-server`, a real loopback listener.

## Verification

- `builds/check.sh` exit 0 over all 10 tools, including `bash -n`,
  `shellcheck -S warning` and `actionlint` on every rendered tree, and the
  shared verdict's self-test.
- Existing forks re-render with only the expected delta: the generalized
  pin-job tag check and the no-op override step. The GitHub checkout step and
  the release-API call render byte-identical.
- Both new binaries built locally with the declared package/ldflags/version
  arg: `weed version` prints the pinned commit; `gitea-runner --version`
  prints `v5.0.0`. act_runner's `cache-server` was started against the shipped
  `config.yaml` and logs `listening on http://127.0.0.1:18088` — i.e. the host
  pin removes the probe.
- govulncheck was rebuilt under each toolchain before use (a `go1.26`-built
  govulncheck cannot type-check `go1.27` sources), matching what the audit job
  does.

Not verified here, and deliberately: the no-egress harnesses need Linux
`unshare`/`strace` or macOS `sandbox-exec` with the artifact in place, so they
run on the fork CI, not in this repo's gate.
