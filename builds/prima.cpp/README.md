# prima.cpp source admission — blocked

Sprint 436 / Story #2029 / `17ba375e3318`. No engine was built, signed,
published, installed or executed by this change. This directory deliberately
has **no `tool.env`**: it is not yet an admissible input to `builds/render.sh`.
The shared stable-release gate must not be relaxed to admit this candidate.

## Exact source evidence (2026-10-13)

| Source | Exact revision | Result |
|---|---|---|
| [qiangli/prima.cpp candidate](https://github.com/qiangli/prima.cpp/commit/f4ea9845dc91a18793921492e50ef23ec799e93e) | `f4ea9845dc91a18793921492e50ef23ec799e93e` | Root LICENSE is MIT; **no stable release tag** |
| [Existing shard-binaries tag](https://github.com/qiangli/prima.cpp/releases/tag/shard-binaries) | `1ea4707ea41dd4c89088c9c045d0049907f1d201` | Makefile links `-lzmq`; not the permissive nng source |
| [nng v1.9.0](https://github.com/nanomsg/nng/tree/v1.9.0) | `29aff2bacc9e2cfa6dcc52155d2816582617f918` | MIT LICENSE.txt verified; vulnerability audit still pending |

Candidate LICENSE SHA-256:
`0ed0eea0d0f31b7f07774a7cac64f9fd20351faf51fab447e9444734448d0452`.
NNG LICENSE.txt SHA-256:
`40138c116893285687962cf132b1a9cb545a502d0626f19a9103e34221cd58fd`.

The release API reports only `shard-binaries` and one asset,
`prima-darwin-arm64.tar.gz`. The tag predates both fork commits:
`ca4b56adceae0434082b9ec85936e2fb40d1d058` replaces ZeroMQ with nng and
changes the fio failure path; `f4ea9845dc91a18793921492e50ef23ec799e93e`
repairs the nng parallel-build dependency. The combined diff changes six
files, 579 insertions and 18 deletions. We did not download or execute the
release asset, and cannot infer its source from its mutable release name or
outpost's `nng-3` VERSION string.

The README's old `Lizonghang/prima.cpp` API returns HTTP 404. A repository
search finds the official [OpenCPIL/prima.cpp](https://github.com/OpenCPIL/prima.cpp),
whose tags and releases APIs both return empty arrays (main observed at
`288df6a2cc2120387e12d6f40798f30cc525c9ea`). The mirror's description identifies
[gitee.com/zonghang-li/prima.cpp](https://gitee.com/zonghang-li/prima.cpp) as
canonical; `/api/v5/repos/zonghang-li/prima.cpp/tags` and `/releases` also
return empty arrays. Neither provides the required official stable tag.

## Remaining audit and build work

- Obtain a policy-admissible official stable tag and review the fork's
  divergence. A locally invented version tag is not an upstream release.
- Enumerate the full linked and runtime graph. The Makefile fetches nng by
  tag on demand; a future build must prefetch and verify the exact commit,
  build it statically with TLS disabled, and deny network during compilation.
  The candidate still contains cppzmq headers, so file presence alone does
  not establish which code is linked. Check link maps and dynamic dependencies.
- Decide whether to include HiGHS (MIT, `USE_HIGHS=1`, needed for the upstream
  automatic scheduler). Pin, audit and source-build it if selected; do not
  silently drop scheduling behavior to make the build pass.
- `common/profiler.cpp:1009` invokes `fio` through `system()` and PATH. Its
  soft failure does not remove this execution path. Resolve it explicitly
  before running on user hosts; do not bundle an unaudited fio executable.
- Inventory vendored headers including nlohmann/json 3.11.3 (MIT), cpp-httplib
  0.15.3 (MIT), stb_image 2.30 (dual MIT/public domain), and ggml/llama code.
  These are source observations, **not a vulnerability audit or complete SBOM**.
  A scanner must account for vendored C/C++ files, not merely package locks;
  fail on high/critical advisories and unknown runtime license coverage.
- Disable model URL download support (`LLAMA_CURL`), review the embedded
  server UI's external resources, and prove no unsolicited outbound traffic
  with a real local model. Observe startup, idle, inference and ring formation
  with explicitly configured peers on each supported platform. `--help`,
  missing-model exits, or denied-network execution alone are not proof.
- Assemble `llama-server`, `llama-cli`, VERSION, notices, runtime dependency
  inventory and per-platform SBOM. Darwin ad-hoc code signing, if required by
  the loader, happens **in CI before archiving and Sigstore signing**, never
  by mutating the downloaded binary on the consumer host.

## CI and consumer coordination

`.github/workflows/prima-source-admission.yml` is hosted source inspection
only. It checks exact commits, pristine trees and license bytes; it never
configures or executes upstream code. Its unsigned review JSON is **not** an
engine, SBOM, audit, provenance attestation or release authorization. It has
read-only contents permission and no OIDC, signing or publication job.

Proposed native matrix: darwin/arm64, darwin/amd64, linux/amd64, linux/arm64,
subject to the manager's outpost requirements. Windows is not supported by
this candidate; the fork's separate `win-build` branch is not an approved
substitute. Do not fall back to upstream assets for unsupported platforms.

After admission, use the shared `builds/render.sh` / `builds/check.sh`
pipeline with custom C/C++ setup/build/audit fragments and per-platform egress
gates. Require every gate before signing. Review the concrete workflow and
protect publication before installing or tagging it: the existing shared
workflow publishes automatically on a matching tag push.

Proposed consumer asset shape: `prima-<os>-<arch>.tar.gz`, plus its
`.sigstore.json` bundle, signed SHA256SUMS, PIN, notices and SBOMs. For a future
reviewed release tag `T`, verify the exact workflow identity
`https://github.com/qiangli/prima.cpp/.github/workflows/bashy-build.yml@refs/tags/T`
and issuer `https://token.actions.githubusercontent.com`, then the archive
SHA-256 **before extraction/execution**. `T` and all artifact hashes remain
unset; there is no approved download URL. Do not trust cached `nng-3`
executables or PATH copies. The manager owns the separate outpost binmgr work;
this change edits no consumer code.

## Verification and delivery status

Local source reads only; no Podman/OCI on Dragon. Targeted commands:

```sh
python3 -B builds/prima.cpp/check_test.py
python3 -B builds/prima.cpp/check.py --source /path/to/prima --nng /path/to/nng
python3 -B builds/prima.cpp/check.py --require-release  # must exit 1
actionlint .github/workflows/prima-source-admission.yml
```

Wrong pins, altered licenses and dirty trees are rejected by bounded offline
tests. Source inspection passes on the exact candidate/NNG checkouts above.
`--require-release` refuses as intended. **CI run: none. Artifact signatures:
none.** The worker is forbidden to push, so hosted execution also awaits the
manager installing the reviewed commit. Story acceptance remains blocked.
