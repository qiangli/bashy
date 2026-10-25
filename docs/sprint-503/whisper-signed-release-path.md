# Whisper six-target signed release path (S503.6)

This is a **candidate path**, not product admission. Native build run
[`40210246732`](https://github.com/qiangli/bashy/actions/runs/40210246732)
produced all six CPU binaries. Its Windows linker maps and loaded archive
members are retained. The build reports still say `product_admitted: false`.

## Release source and layout

Create a reviewed, immutable `qiangli/bashy` tag matching
`whisper-cpp-v1.9.5-bashy.<revision>` at the exact source commit. Run
`.github/workflows/s503-whisper-release.yml` with `workflow_dispatch` **on
that tag**, with the matching `release_tag` input. The workflow calls the
native six-runner build at the tagged commit, verifies each native binary's
GitHub attestation against the reusable build workflow, tag ref, source
commit, and hosted-runner policy, stages the files below, signs
each file and `SHA256SUMS` with keyless cosign, and verifies every bundle
against the exact tagged release workflow identity. `publish` defaults to
false. Publication also requires each new native report to say
`product_admitted: true`.

The release directory has exactly these data assets plus one
`<name>.sigstore.json` bundle for each data asset and for `SHA256SUMS`:

| Asset | Origin |
| --- | --- |
| `whisper-cli-linux-amd64` | `ubuntu-24.04` native build |
| `whisper-cli-linux-arm64` | `ubuntu-24.04-arm` native build |
| `whisper-cli-darwin-amd64` | `macos-15-intel` native build |
| `whisper-cli-darwin-arm64` | `macos-15` native build |
| `whisper-cli-windows-amd64.exe` | `windows-2025` native build |
| `whisper-cli-windows-arm64.exe` | `windows-11-arm` native build |
| `WHISPER-NOTICES.txt` | reviewed full third-party notices |
| `whisper-release-manifest.json` | source commit, six binary digests, report digests, notice digest |
| `whisper-build-evidence.tar.gz` | six reports and runner records; two Windows maps and inventories |
| `SHA256SUMS` | hashes of the preceding nine assets |

The staging script verifies the runner/source commit, pinned whisper.cpp
archive, build policy, binary bytes and size, and both Windows linker maps
against their inventories before naming release assets. The maps and build
reports are retained in the signed release evidence archive; the signed
release manifest records each report digest. Native binary attestations are
also stored by GitHub for the tagged build run.

## Installation pin

Yoke `WhisperCppTool` names the planned Bashy release tag, exact release
workflow identity, six per-asset Sigstore bundles, and signed `SHA256SUMS`.
It requires the three supplementary files above in the digest manifest. After
a release is published and all bytes are reviewed, obtain the six SHA-256
values from that **tagged release's signed** `SHA256SUMS`, verify the sums
bundle and each asset bundle against
`https://github.com/qiangli/bashy/.github/workflows/s503-whisper-release.yml@refs/tags/<tag>`
and the GitHub Actions issuer, then commit those six values in Yoke
`pkg/binmgr/pins.go` under
`whisper-cpp@<tag>/<os>/<arch>`. Do not copy hashes from branch run
`40210246732`: the tagged build may produce different bytes. Until pins are
committed, `EnsureWhisperCpp` refuses the install.

## Open admission gates

- Complete and review `examples/transcribe/dependencies/WHISPER-NOTICES.txt`
  from the exact linked components. The conservative Windows inventory in
  `docs/sprint-503/evidence/windows-static-whisper-license-review.md`
  identifies the LLVM, MinGW-w64, whisper.cpp, and GGML notice candidates;
  Cephes and other unresolved license points still need disposition.
- Obtain bare-host evidence for the Windows Python and native executable
  prerequisites and all six target installs. CI runner success alone does
  not establish product admission.
- Change the native build's `product_admitted` assertion only after the
  license and bare-host evidence has been reviewed. The current workflow
  explicitly asserts `false`; a requested publication therefore fails closed.
- Preserve the signed tagged release's native build attestations alongside
  the release evidence archive outside the 14-day Actions artifact window.
