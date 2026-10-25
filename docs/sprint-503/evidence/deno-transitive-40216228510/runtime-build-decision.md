# S503.6 JavaScript runtime build decision

**Decision for the next experiment:** pilot QuickJS-NG `v0.17.0` as the
external yt-dlp EJS runtime on two native targets, then on all six if the
pilot passes. Keep the pinned Deno 2.9.8 path available for existing controlled
evidence. **Neither path is admitted for product installation by this memo.**
The six official Deno subjects remain blocked as recorded in
[the independent review](source-bundle-independent-review.md).

## First-party Deno 2.9.8 path

The pinned Deno source commit is
[`fe7ed012d92c642fd8e5607ac5a45d517538b5c5`](https://github.com/denoland/deno/tree/fe7ed012d92c642fd8e5607ac5a45d517538b5c5).
Its official `deno_src.tar.gz` has SHA-256
`b57d0281c1647effc735f242ba52bc409747fc6ed6ad4946e76f062c67e113eb`;
`Cargo.lock` is
`36aab78913808a9fb2e5a3a3a27a74a1a1045011133f28beaac6eabf57fc9895`,
and `rust-toolchain.toml` selects Rust **1.95.0**. The lockfile lists the
`v8` Rust crate at **150.4.0** with registry checksum
`42a978ff11f15b24e5c05a7123cf2b68f41e763546699781a924ef4e2cf43a49`.
These are source inputs, not a linked-component manifest.

Deno's [release workflow](https://github.com/denoland/deno/blob/fe7ed012d92c642fd8e5607ac5a45d517538b5c5/.github/workflows/ci.generated.yml)
uses a locked release Cargo build of `deno` and then performs platform-specific
strip/sign/package steps. The [Deno build guide](https://github.com/denoland/deno/blob/fe7ed012d92c642fd8e5607ac5a45d517538b5c5/.github/CONTRIBUTING.md)
requires the source submodules and offers `V8_FROM_SOURCE=1`. The
[`rusty_v8` build guide](https://github.com/denoland/rusty_v8) says ordinary
Cargo builds download a prebuilt V8 static library; source builds require
`V8_FROM_SOURCE=1` and more build dependencies. A first-party Deno build that
uses the prebuilt V8 archive still needs a separate, byte-specific V8 archive
and notice closure. A source V8 build must pin its own repository/dependency
inputs and record the static archive/object linkage.

For each target, a valid new executable subject needs the exact Cargo
resolution/features, compiler and linker versions/options, V8 source or
archive hash, native archive members and linker map, generated JavaScript and
snapshot inputs, direct OS imports, final strip/sign transform, all licenses
and required notice text, plus a final executable hash. Keep manifests and
notices as release artifacts and review unknown/copyleft terms before any
product download. `Cargo.lock`, `cargo tree`, and a source-wide license scan
cannot substitute for this final-byte linkage.

**Hash-identical rebuild:** treat matching the six upstream executable hashes
as unlikely. This is an inference from the unpublished target-specific linker
inputs and platform strip/sign transformations, not a failed rebuild result.
Plan to mint and review **six new executable hashes**. An upstream
per-executable attestation would be a shorter route for the old hashes.

## Smaller supported runtime

The pinned [yt-dlp 2026.10.21 QuickJS provider](https://github.com/yt-dlp/yt-dlp/blob/2026.10.21/yt_dlp/extractor/youtube/jsc/_builtin/quickjs.py)
supports `--js-runtimes quickjs:/path/to/qjs`. The
[yt-dlp EJS guide](https://github.com/yt-dlp/yt-dlp/wiki/EJS) lists QuickJS
and QuickJS-NG as supported, alongside Deno (recommended), Node and Bun.
It warns that QuickJS writes challenge scripts to temporary files and can be
slower, especially older versions. The pinned provider identifies
QuickJS-NG **0.12.0** as its recommended performance floor. QuickJS does not
inherit Deno's restricted-permissions execution path. This is a real runtime
and temporary-file security change to review, not just a smaller download.

[QuickJS-NG `v0.17.0`](https://github.com/quickjs-ng/quickjs/releases/tag/v0.17.0)
is pinned to commit `6d46d07d04041b40f4f49eaa7fdebe44c314c699`; its
[license file](https://github.com/quickjs-ng/quickjs/blob/v0.17.0/LICENSE)
is MIT. Its [build guide](https://quickjs-ng.github.io/quickjs/building/)
uses CMake and explicitly supports Windows builds. The default CMake option
does not enable mimalloc; leave optional libraries disabled unless separately
reviewed. The release has native `qjs` assets for both Linux and macOS
architectures and Windows x64, but **no Windows ARM64 asset**. Official asset
sizes are about 1.2–2.6 MB, versus 81–99 MB for the six reviewed Deno
executables. These are size observations, not license or performance proof.

A first-party six-target QuickJS-NG build is likely a smaller closure task
than Deno plus V8: its CMake target has a much smaller source/link graph and
no Rust crate/V8 download chain. That is an engineering inference, still
subject to source-file and generated-table notices, native C runtime/toolchain
links, and a Windows ARM64 build. Do not adopt official prebuilt bytes on five
targets while treating an unreviewed sixth build as equivalent. New native
build subjects should each carry their own exact provenance and notice
manifest. The existing adapter can select a different runtime without a core
pipeline change, but its policy/hash check and declaration must be updated
and retested.

## Native jobs and bounded spend

Use the already proven native [GitHub-hosted labels](https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job):

| Target | Runner |
| --- | --- |
| linux/amd64 | `ubuntu-24.04` |
| linux/arm64 | `ubuntu-24.04-arm` |
| darwin/amd64 | `macos-15-intel` |
| darwin/arm64 | `macos-15` |
| windows/amd64 | `windows-2025` |
| windows/arm64 | `windows-11-arm` |

Build times have **not been measured**. Start with a 30-minute QuickJS-NG
pilot on Linux x64 and Windows ARM64; stop if either cannot produce a native
`qjs` and a reviewed link/input record. A six-job, 30-minute cap would consume
at most **180 raw runner-minutes**. For Deno, first pilot Linux x64 and Windows
ARM64 with a 120-minute cap each, disk/memory preflight, and a fail-closed
`V8_FROM_SOURCE=1` build. If both succeed and notices can be closed, a later
six-job 120-minute cap would consume at most **720 raw runner-minutes**. These
are experiment budgets, not expected build durations or a promise the V8 build
fits the standard runners' 14 GB disks; [GitHub lists their native memory and
disk limits](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
The `qiangli/bashy` repository was public at review time; GitHub says
standard hosted runners for public repositories have no per-minute charge.
If the repository becomes private, the [published standard rates](https://docs.github.com/en/billing/reference/actions-runner-pricing)
would put six 120-minute jobs at about **US$18.60**, before storage and
assuming all jobs consume their full cap. Confirm billing state before launch.

## Gate before changing the selected runtime

1. Pin the runtime source commit/archive and every external build input; build
   natively on all six runners, record host/process architecture and final
   hashes, inspect full link maps and required notices. Keep admission false
   on missing Windows ARM64/toolchain or unknown license evidence.
2. Prove the pinned yt-dlp `2026.10.21` zipapp and bundled EJS `0.8.0` solve
   a controlled EJS challenge fixture with `--no-remote-components` using the
   exact QuickJS executable, including cleanup/cancellation of the temporary
   JavaScript files. Compare runtime and output to Deno on the same fixture.
3. Then rerun the actual operator canonical and shortened URL acquisition
   and downstream transcript/export gates on six native hosts. A successful
   controlled test does not waive the current YouTube bot-sign-in failure or
   the human-reviewed spoken-reference requirement.

No CI/build, host runtime, container, or OCI action was performed for this
decision memo. Product admission remains false for both candidate routes.
