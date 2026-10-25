# Deno 2.9.8 official executable linkage disposition

**Scope:** the six official release executable members reviewed in [run
40216228510](https://github.com/qiangli/bashy/actions/runs/40216228510),
not a later Deno build or a source dependency graph. The independent
[`review.json`](review.json) establishes six archive and executable hashes,
native identity, and Deno/EJS runtime checks. Its transitive verdict remains
`BLOCKED`, with `product_admitted: false` for every target.

| Target | Official archive SHA-256 | Executable SHA-256 |
| --- | --- | --- |
| linux/amd64 | `e890975bc2549807bbcc0b584ffafda72cc7c93349dd99c80483dd784f6c4609` | `368bca08999e02c6911b8f248167b582276b1f547db5712b6e736d7eda1dbda7` |
| linux/arm64 | `d49596c20b9788cf8f5d8ddf644103d4bde56ed239753f0bdb92ba9361d51905` | `b484f780e5149ca05c673f1f79f7fb466f12a2295933bdad917af4a8a5c020e8` |
| darwin/amd64 | `4b2f5edd2639b7470f034238b321d7002d8dfb4726d8558f52c17afac370eb2c` | `5d206ba69742549cc161db06e86f1d93c5622e1dbc8ba9d15a618e538fbc92f4` |
| darwin/arm64 | `9f0bf660393da2eaff044cdc7444cb08c73f235849b61d224d825a194ce7c98e` | `d1e99265e5e6ee096c5780d88ff7d3e6cfdf33b1953a118b4889cab8ca6980ba` |
| windows/amd64 | `cba84b26e0df5ae7f74fc66b86a95b866872a86292ebd84fa1e959927edd93bf` | `924397800533d3eb0ed0c889b20ad70e18b970d93acd8135bc043f425e528266` |
| windows/arm64 | `b413b92b735b50c198e8fe3b85af1aa50e29d5f72e229aab701d132286e14cef` | `40d161bc35657cc7432a2e3832290922939cd63fddd29f66db2261d291e42478` |

## What the upstream record supplies

The [official v2.9.8 release](https://github.com/denoland/deno/releases/tag/v2.9.8)
publishes checksums for the six archives. Deno's [installation
documentation](https://docs.deno.com/runtime/getting_started/installation/#manual-download)
says each platform archive contains one executable. The [pinned source
workflow](https://github.com/denoland/deno/blob/fe7ed012d92c642fd8e5607ac5a45d517538b5c5/.github/workflows/ci.generated.yml)
has six release build jobs. They use `cargo build --release --locked -p deno
-p denort -p test_server --bin deno --bin denort --bin test_server
--features=deno/panic-trace`; the macOS jobs strip and sign `deno`, and the
Windows jobs sign `deno.exe`, before archiving. Its GitHub release upload list
names archives, checksums, `deno_src.tar.gz`, declarations and patches. It
does not name a component SBOM, linker map, or third-party notice bundle.
The [source LICENSE.md](https://github.com/denoland/deno/blob/fe7ed012d92c642fd8e5607ac5a45d517538b5c5/LICENSE.md)
is Deno's MIT notice; it does not identify the licenses of every component
in any executable.

Those official sources explain how the release is intended to be built, but
the retained evidence does not bind a resolved component graph or native
linker inputs to *these executable hashes*. The collector's 1,130
`Cargo.lock` entries are workspace candidates: 1,048 registry entries and
82 workspace/unknown entries, with no license text or binary linkage. The
archive member scan found zero notices. A target-specific `cargo tree`,
`cargo metadata`, or `--unit-graph` result would narrow source candidates,
but still would not identify which native archive members, generated
objects, bundled JavaScript, or V8 inputs made the published bytes.

[`cargo-auditable`](https://github.com/rust-secure-code/cargo-auditable)
documents a build-time embedded Rust dependency list. The Deno workflow uses
ordinary `cargo build`, and no embedded list has been demonstrated in the
six reviewed executables. A section scan can test for that list, but a
negative result only confirms this gap; a positive result still requires
native and bundled-script notice review. Rebuilding with cargo-auditable
would describe the **new** binaries, not these six official binaries.

## Exact unresolved mapping

All six executable hashes lack an independently verified mapping to:

1. The Rust crates actually linked into that executable, including their
   exact source checksums, license expressions and required notice text.
2. V8 and other native/static archive members actually linked, with source
   revisions, archive/object provenance, licenses, and required notices.
3. Bundled JavaScript, generated snapshots, TypeScript and any copied
   third-party data, with exact source and notice provenance.
4. A complete, applicable notice bundle bound to the executable hash. No
   notice is present as a separate member of the reviewed archives.

The platform provider question is separate from compiled-in components:

| Targets | Direct-import evidence still needing classification |
| --- | --- |
| linux/amd64, linux/arm64 | Resolve the ELF interpreter (`ld-linux-x86-64.so.2` or `ld-linux-aarch64.so.1`) separately; the `ldd` parsing miss in the collector is not proof of an absent loader. Retain provenance for the other resolved shared providers. |
| darwin/amd64, darwin/arm64 | Resolve `otool -L` system library/framework names through the native dyld shared cache or mark the provider unverified; `Path.is_file() == false` is not proof that a provider is absent. |
| windows/amd64, windows/arm64 | Resolve the `api-ms-win-core-synch-l1-2-0.dll` API-set contract to its native host, or leave it unverified. The separately hashed VC runtime DLLs are CI host dependencies, not embedded Deno notices. |

## Evidence required to close this blocker

**Exact upstream evidence request:** for each of the six executable SHA-256
subjects in the table, provide the release build run and source revision;
the resolved target/features Rust unit graph; the final linker map and
static archive member list; V8, generated snapshot and bundled-script input
identities; the strip/sign transformation record; and a component license
and notice manifest whose subject is that final executable hash. State which
components are shipped inside the executable and which are supplied by the
host OS. A release-wide source tree or one manifest for all six targets
cannot answer the target-specific linkage question on its own.

For **these six official executable hashes**, obtain an upstream-produced
or independently verified per-target build record whose subject is the
post-strip/post-sign executable hash. It must identify the source commit,
locked Cargo resolution and features, final Rust compilation units, native
linker inputs and loaded archive members, generated/snapshot/JavaScript
inputs, and any signing or packaging transformations. A reproducible build
with an exact executable hash match could supply the linkage instead, but
matching source version or runtime output is insufficient. Then collect the
licenses and notice texts from the exact source revisions, review any
non-permissive or unknown terms against the distribution policy, and publish
a notice manifest keyed to each subject hash. Resolve the direct native
providers by target without treating ordinary system dependencies as
embedded code.

An upstream component/notice attestation keyed to each published executable
would be the shortest route. A first-party audited source build with retained
link maps and notices is an alternative product path only after its **new**
bytes are pinned and verified on all six targets; it cannot retroactively
admit the official executables above. Until one path supplies the missing
links and license/notice review, the six transitive verdicts remain
`BLOCKED` and `product_admitted: false`.
