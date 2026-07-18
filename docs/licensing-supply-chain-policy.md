# Licensing & supply-chain policy (bashy)

bashy is **permissive open source**. Everything bashy compiles in, embeds, links,
or vendors must be permissive. This doc is the rule of record; the terse form
lives in `CLAUDE.md` §Third-Party Libraries.

## 1. Compiled-in / embedded / linked / vendored → permissive only

**BSD, MIT, or Apache-2.0 only.** No GPL, LGPL, MPL, SSPL, BSL, Commons-Clause,
ELv2, or proprietary — anything whose license could **propagate** to bashy. This
covers Go module deps, embedded blobs (`*_embed`), and any vendored source.

- **cgo-free core**: releases build `CGO_ENABLED=0`. cgo is only for opt-in,
  non-core/host pieces where no pure-Go option exists.
- Every compiled-in third party is recorded in `THIRD_PARTY_LICENSES` with its
  license (Apache-2.0 additionally requires stating changes).

## 2. Runtime download + exec is NOT bundling

Tools bashy **downloads and runs as separate processes** — the Tier 2/3 dispatch
rungs (`docs/bashy-execution-path.md`): podman, ollama, gh, loom, act, rclone,
zot, seaweedfs, kopia, and the conformance test suites — are **not** bundled,
linked, or vendored. They are separate programs on their own licenses, fetched at
runtime into a cache and exec'd as subprocesses; **no license propagates to
bashy** (the same posture that lets the harness fetch a GPL test suite at runtime
without vendoring it). Prefer permissive tools anyway, and record the source.

**Strong-copyleft edge case (AGPL): the SearXNG rung.** The `bashy search` web
ladder's keyless last rung is a self-hosted **SearXNG** (AGPL-3.0), run via
`bashy podman`. AGPL is the strictest copyleft — its §13 adds a *network* clause
GPL lacks — so it is worth spelling out why it is still a clean download+exec:
- bashy **links/vendors nothing** — its only contact is an HTTP call to the
  instance's `/search?format=json`; a separate process across a socket is not a
  derivative work, so no copyleft crosses the boundary.
- bashy runs the **unmodified official image** bound to **localhost**, queried
  only by bashy on the same host. AGPL §13's source-offer obligation fires only
  when you **modify** the program **and** serve it to **remote users** — neither
  holds, so §13 stays dormant.
- bashy does not **convey** SearXNG: the user's own podman pulls the official
  image at runtime (like `ollama pull`), so bashy never distributes AGPL code.

The AGPL therefore stays on that separate program, not on bashy. The discipline
that keeps it clean, and must hold: **pull the official image unmodified**
(configure via a mounted `settings.yml` only — configuration ≠ modification),
**bind localhost**, and **never vendor it, fork-and-ship it, or expose it to
third parties.** The rung is **off by default** (enabled only by setting
`BASHY_SEARXNG_URL`); the keyed backends (Brave free tier, etc.) are the
permissive-clean default per the "prefer permissive anyway" rule above.

## 3. Absolutely required + no permissive substitute → build from source

If a component is genuinely required and **no permissive substitute exists**, do
**not** ship a non-permissive prebuilt binary. Instead **build it from (permissive)
source using bashy's self-provisioning toolchain** — `bashy go` / `bashy cmake` /
`bashy clang`, each binmgr-fetched (download → sha256-verify → cache) so a bare
box needs nothing preinstalled. Build either:
- **in bashy CI**, publishing the result as a downloadable release asset, or
- **on demand on the host** (first use builds + caches).

Building from permissive source keeps the supply chain clean and avoids
redistributing anything non-permissive. (This is why bashy already carries the
`bashy go/cmake/clang` provisioners — proven building the userland from source on
a bare Windows box.)

## 4. Applied: the self-contained engines

`bashy podman` / `bashy ollama` (and the mac VM helpers vfkit/gvproxy) must "just
run" on a bare box with no manual install — under this policy:

| component | upstream license | how bashy ships it |
|---|---|---|
| podman | Apache-2.0 | built from Go source in CI → release asset → binmgr-fetch → exec |
| gvproxy (gvisor-tap-vsock) | Apache-2.0 | same |
| vfkit | Apache-2.0 | same |
| ollama | MIT | built from source (ggml/llama.cpp = MIT) → asset → fetch → exec |

- **All permissive**, so download+exec is clean — and bashy **builds them from
  source** (the `scripts/embed-*.sh` already do), which is the §3 posture even
  though it isn't strictly required here.
- The lean binary **never embeds** them (stays small, `CGO_ENABLED=0`); it
  binmgr-fetches the bashy-built blob from bashy's own release on demand (Tier 2)
  or falls back to a host/PATH copy (Tier 3), and only if neither exists points
  the user to install or a paired host node (Tier 4) — never "rebuild bashy".
- Rationale for **bashy-built blobs over raw upstream prebuilts**: (a) a curated,
  all-permissive supply chain we control and can attest; (b) avoids upstream's
  fat multi-GB CUDA/rocm bundles; (c) `CGO_ENABLED=0` where possible.

## Checklist for adding any new dependency or tool

1. Is it compiled-in/embedded? → must be BSD/MIT/Apache-2.0; add to
   `THIRD_PARTY_LICENSES`.
2. Is it download+exec only? → not bundled, but prefer permissive; record the
   source in the relevant `external/<tool>` or verb.
3. Required with no permissive substitute? → build from permissive source via
   `bashy go/cmake/clang`; never ship a non-permissive prebuilt.
4. In doubt, keep it out and open the question.
