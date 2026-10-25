# S503.6 Deno and offline EJS admission

This record covers the external JavaScript runtime and YouTube challenge scripts
used by the S503 source adapter. It is **dependency evidence**, not a native
transcription or operator video result. The six-runner workflow
`.github/workflows/s503-deno-admission.yml` runs on a push to
`s503-contract-integration` that changes this record or that workflow. It also
has manual dispatch after the workflow reaches the default branch. A branch
push is the first usable trigger for a newly added workflow.

## Pinned artifacts

All Deno values below are SHA-256. The archive values agree with each official
`*.zip.sha256sum` release asset and the executable values with each official
`*.sha256sum` asset. I fetched the **small checksum files only** on 2026-10-25,
verified each checksum file's bytes against its GitHub release API asset digest,
and parsed the published payload hash. The Windows checksum files use
PowerShell `Algorithm`/`Hash`/`Path` fields rather than `sha256sum` lines.
The workflow downloads and independently hashes the actual archive and
executable on each matching native runner. Its result must be inspected before
these are treated as six passed dependency paths.

| Runner | Deno v2.9.8 archive | Archive SHA-256 | Executable SHA-256 |
| --- | --- | --- | --- |
| `ubuntu-24.04` | `deno-x86_64-unknown-linux-gnu.zip` | `e890975bc2549807bbcc0b584ffafda72cc7c93349dd99c80483dd784f6c4609` | `368bca08999e02c6911b8f248167b582276b1f547db5712b6e736d7eda1dbda7` |
| `ubuntu-24.04-arm` | `deno-aarch64-unknown-linux-gnu.zip` | `d49596c20b9788cf8f5d8ddf644103d4bde56ed239753f0bdb92ba9361d51905` | `b484f780e5149ca05c673f1f79f7fb466f12a2295933bdad917af4a8a5c020e8` |
| `macos-15-intel` | `deno-x86_64-apple-darwin.zip` | `4b2f5edd2639b7470f034238b321d7002d8dfb4726d8558f52c17afac370eb2c` | `5d206ba69742549cc161db06e86f1d93c5622e1dbc8ba9d15a618e538fbc92f4` |
| `macos-15` | `deno-aarch64-apple-darwin.zip` | `9f0bf660393da2eaff044cdc7444cb08c73f235849b61d224d825a194ce7c98e` | `d1e99265e5e6ee096c5780d88ff7d3e6cfdf33b1953a118b4889cab8ca6980ba` |
| `windows-2025` | `deno-x86_64-pc-windows-msvc.zip` | `cba84b26e0df5ae7f74fc66b86a95b866872a86292ebd84fa1e959927edd93bf` | `924397800533d3eb0ed0c889b20ad70e18b970d93acd8135bc043f425e528266` |
| `windows-11-arm` | `deno-aarch64-pc-windows-msvc.zip` | `b413b92b735b50c198e8fe3b85af1aa50e29d5f72e229aab701d132286e14cef` | `40d161bc35657cc7432a2e3832290922939cd63fddd29f66db2261d291e42478` |

Archive coordinates use
`https://github.com/denoland/deno/releases/download/v2.9.8/<archive>`.
The [immutable Deno release](https://github.com/denoland/deno/releases/tag/v2.9.8)
and its [release API](https://api.github.com/repos/denoland/deno/releases/tags/v2.9.8)
publish the assets and their digests. The existing Darwin arm64 and Windows
amd64 helper pins match the corresponding rows. The workflow pins the
checksum-file digests as well as the payload hashes, so a changed upstream
manifest fails rather than silently changing the admitted bytes.

The separate [yt-dlp 2026.10.21 `yt-dlp` zipapp](https://github.com/yt-dlp/yt-dlp/releases/tag/2026.10.21)
has SHA-256 `b7af77f686e95f7845ce8d09d21957718febb8b48b30555adb9dd2292ad7bf14`.
That matches its [published `SHA2-256SUMS`](https://github.com/yt-dlp/yt-dlp/releases/download/2026.10.21/SHA2-256SUMS)
(manifest SHA-256 `ea1a0e0274b6e8451d9c78d6645edd5a6241465e08a2a615f08a5e4221f8b62f`).
The zipapp is expected to contain `yt_dlp_ejs` version `0.8.0` and both
offline solver files. The [EJS 0.8.0 release](https://github.com/yt-dlp/ejs/releases/tag/0.8.0)
publishes `yt.solver.core.min.js` SHA-256
`18da6ce0758b416e7ae645084f4f8801f9f9d59d6c477c05eaa0ff94ebd8cc00`
and `yt.solver.lib.min.js` SHA-256
`c55987fe697e5b9ee18830163f7af85327e9bb5c3e674b969d38c8d205eaa577`.
I checked those bytes in the small 0.8.0 wheel, whose release digest is
`79300e5fca7f937a1eeede11f0456862c1b41107ce1d726871e0207424f4bdb4`.
The workflow checks the same paths and hashes inside the **actual pinned
yt-dlp zipapp**, then runs its version command. The source adapter passes
`--no-remote-components`, an explicit Deno path, and no other JS runtime to
yt-dlp; it will not fetch EJS at extraction time.

[yt-dlp's EJS setup guide](https://github.com/yt-dlp/yt-dlp/wiki/EJS)
requires Deno 2.3.0 or newer for this path and says to use `deno`, not
`denort`. v2.9.8 satisfies the version requirement. The workflow checks
`deno --version` and executes a small local `deno eval` on every target.

## External CI runtime identity

[Run 40196010350](https://github.com/qiangli/bashy/actions/runs/40196010350)
passed all six Deno/yt-dlp/EJS dependency jobs. Its reports recorded Python
3.12.15 on Linux and 3.12.10 on macOS/Windows, but did not hash Python or
record Windows VC runtime files. The updated workflow fixes those same Python
patches per target. A new run will also run `probe_runtime.py`, recording the
exact `actions/setup-python@v5` tool-cache
executable path, patch version and SHA-256. On Windows it records the native
`System32` copies of `MSVCP140.dll` and `VCRUNTIME140.dll`, with file versions
and SHA-256. A missing or unreadable dependency fails the job. These are
external hosted-runner prerequisites and are never copied into the artifact.

The six-target transcription gate requires a reviewer to place the observed
Python identities for all six labels and VC identities for both Windows labels
in its admission record. It re-probes the target runner and rejects changed
versions or bytes before acquisition. This binds the CI test environment; it
does not establish a distributable CPython installation or permission to
redistribute Microsoft runtime files. The current admission record is absent,
and these new runtime identities have not yet been collected natively.

## License boundary

The adapter downloads and executes Deno and the yt-dlp zipapp as **separate
programs**; neither is linked, embedded, or vendored into bashy. That is the
runtime-acquisition boundary in
[bashy's licensing policy](licensing-supply-chain-policy.md).
The upstream [Deno v2.9.8 license](https://github.com/denoland/deno/blob/v2.9.8/LICENSE.md)
is MIT. [yt-dlp's tagged license](https://github.com/yt-dlp/yt-dlp/blob/2026.10.21/LICENSE)
is Unlicense; its release notes disclose ISC and MIT code in the zipapp.
The [EJS 0.8.0 package declaration](https://github.com/yt-dlp/ejs/blob/0.8.0/pyproject.toml)
states `Unlicense AND MIT AND ISC`, and its
[license text](https://github.com/yt-dlp/ejs/blob/0.8.0/LICENSE)
is the Unlicense. These are permissive declarations for the exact source
versions. This review does not assert a full transitive binary SBOM for Deno.

## Evidence and remaining gate

Each runner uploads `deno-ejs-admission.json` even if a check fails. A PASS
requires matching OS/process architecture, no macOS translation, the two
upstream Deno checksum-file digests, archive and executable hashes, a working
Deno 2.9.8 CLI, the published yt-dlp zipapp hash, and matching bundled EJS
0.8.0 solver bytes. The artifact records the candidate commit and observed
versions, sizes and hashes. A failed or missing runner remains a blocked target;
there is no partial aggregate PASS.

The original dependency run passed **6/6 native Deno/EJS checks**. The new
external-runtime identity check has **0/6 native results** until a new run.
S503.6 still needs runtime identity review and a product install policy;
S503.8 still needs the operator video, transcript, timing and proved offline
native pipeline evidence on all six targets. Neither a Deno/EJS PASS nor a
native whisper build alone closes those gates.
