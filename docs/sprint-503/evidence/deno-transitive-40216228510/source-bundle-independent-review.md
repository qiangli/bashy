# Deno 2.9.8 source bundle: independent closure check

**Story:** Sprint #503, S503.6 (`74ed924ba244`). **Verdict:** six official
Deno executables remain `BLOCKED` for transitive license and notice admission.
This check covers source identity only. It does not change the byte/runtime
`PASS` or `product_admitted: false` in [the six-target review](review.json).

The YouTube adapter reads `examples/transcribe/dependencies/deno-runtime.json`
and hashes the selected Deno executable before passing it to yt-dlp. All six
policy hashes exactly match the executable subjects in
[the binary disposition](binary-linkage-disposition.md). The Python zipapp and
its embedded EJS are separate, already reviewed subjects; their notice review
cannot stand in for Deno's compiled content.

## Independently checked official source asset

The [official v2.9.8 release](https://github.com/denoland/deno/releases/tag/v2.9.8)
API enumerated 56 uploaded assets on 2026-10-25. The uploaded
`deno_src.tar.gz` had size **37,035,922 bytes** and GitHub's digest
`sha256:b57d0281c1647effc735f242ba52bc409747fc6ed6ad4946e76f062c67e113eb`.
I downloaded that exact asset and independently recomputed the same SHA-256.
No uploaded asset name contains `sbom`, `notice`, `license`, `spdx`,
`cyclonedx`, `provenance`, or `attestation`. GitHub's automatic source-code and
release-attestation links are separate from those uploaded assets; this is not
a claim that no other upstream evidence exists.

| File inside `deno_src.tar.gz` | Recomputed SHA-256 | Earlier commit-source review |
| --- | --- | --- |
| `deno/Cargo.lock` | `36aab78913808a9fb2e5a3a3a27a74a1a1045011133f28beaac6eabf57fc9895` | exact match |
| `deno/LICENSE.md` | `f62497fffecc0852960c8d3e6934b9db86d16396e9b604072e923892cae3a588` | exact match |
| `deno/cli/Cargo.toml` | `ac2ca211c59b910c2b05ecd9ecdd28b3b6b529a29c8002a19695b75e5a669de4` | exact match |

These matches establish that the official source bundle and the reviewed
release-commit files agree for these three files. They do **not** map any
source file to any of the six official executable hashes. The archive lists
no `deno/third_party/` member, so it cannot by itself provide the native V8
and other third-party source/notice inventory. That absence also does not
prove those components were absent from the build.

## Minimal evidence needed next

Ask Deno's release owner for **one record per executable SHA-256** in the
[six-subject table](binary-linkage-disposition.md), with:

1. Release build job and source commit, locked target/features/unit graph,
   final linker map or equivalent loaded static archive member list, and the
   V8/native and generated JavaScript/snapshot input revisions.
2. Strip/sign/package transformation from build output to the exact published
   executable hash. A record keyed only to `v2.9.8` or `Cargo.lock` is
   insufficient.
3. SPDX/CycloneDX or equivalent component list keyed to that final hash,
   paired with the applicable license expression, required copyright/notice
   text, and whether each component is embedded or an external OS provider.

The release's [source workflow](https://github.com/denoland/deno/blob/fe7ed012d92c642fd8e5607ac5a45d517538b5c5/.github/workflows/ci.generated.yml)
is a useful starting point for locating those build jobs; it is not itself a
per-binary component attestation. A target-specific Cargo graph would narrow
the 1,130 lockfile candidates but would still need native and generated-input
closure. A reproducible, hash-identical rebuild could replace an upstream
attestation. If that is not feasible, an audited first-party build produces
**new** six-target subjects and must carry its own full input maps and notices;
it cannot retroactively admit the six official hashes.

Until one route is reviewed, keep Deno external-only for controlled testing,
pin and rehash the executable before use, and do not turn successful yt-dlp
execution into a product dependency or redistribution verdict.

## Reproduction (source inspection only)

```sh
gh api repos/denoland/deno/releases/tags/v2.9.8 --jq \
  '.assets[]|select(.name=="deno_src.tar.gz")|{name,size,digest}'
gh release download v2.9.8 --repo denoland/deno \
  --pattern deno_src.tar.gz --dir "$scratch"
shasum -a 256 "$scratch/deno_src.tar.gz"
tar -xOzf "$scratch/deno_src.tar.gz" deno/Cargo.lock | shasum -a 256
tar -xOzf "$scratch/deno_src.tar.gz" deno/LICENSE.md | shasum -a 256
tar -xOzf "$scratch/deno_src.tar.gz" deno/cli/Cargo.toml | shasum -a 256
```

The source inspection downloaded no executable and ran no Deno, container,
OCI, or native transcription workload on Dragon.
