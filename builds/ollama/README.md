# Official Ollama runner recipe — release blocked

Sprint #436 / Story #1917 (`2498b5c36ace`). Source qualification is Yoke
[`docs/validation/ollama-runner-source-2018.md` at `5e8c4687`](https://github.com/qiangli/yoke/blob/5e8c46870baadd0eb58e68f73a6a5a4ea66f9102/docs/validation/ollama-runner-source-2018.md).
This recipe pins official `ollama/ollama` **v0.21.2**, peeled commit
`590109c8352e8d5a6206e8909b518a54a2b0a7b8`. The runner source is independent
of Bashy's Go module graph and Yoke's `external/ollama/src` server fork.
No module pin, server gitlink, or runtime fallback is changed here.

`fork/` is generated with `bash builds/render.sh ollama`; its destination is
the dedicated `bashy-build` branch of `qiangli/ollama`, never the fork's
server development branch. This delivery does not install the branch, push
refs, trigger hosted jobs, sign bytes or publish a release.

## Native build contract

| Target | Hosted runner | Compiler | Baseline |
|---|---|---|---|
| Linux amd64 | ubuntu-24.04 | GCC / G++ | CPU, GOAMD64=v1 |
| Linux arm64 | ubuntu-24.04-arm | GCC / G++ | CPU, GOARM64=v8.0 |
| Darwin amd64 | macos-15-intel | Xcode Clang | CPU, macOS deployment target 14.0 |
| Darwin arm64 | macos-15 | Xcode Clang | CPU / built-in Metal, deployment target 14.0 |
| Windows amd64 | windows-2025 | image MinGW64 GCC / G++ | CPU, native PE `.exe` |

The workflow uses a full Git **file tree** checkout, including nested vendor
headers (a shallow history is sufficient). Both HEAD and the peeled tag must
match the pin; dirty trees, missing nested headers and unreviewed gitlinks
fail. Go module ZIPs and Yoke's patched source do not qualify. No generated
main, dispatcher pruning, header relocation or upstream patch is applied.

Go 1.27.2 is explicit (source minimum 1.24.1); `GOTOOLCHAIN=local`,
`GOENV=off`, `GOWORK=off`, empty `GOFLAGS`, and `CGO_ENABLED=1` prevent ambient
workspace/toolchain selection. This supported-toolchain choice follows the
source qualification; vulnerability admission still requires its own gate.
`GOHOSTOS/GOHOSTARCH` must equal the requested target; setting GOOS alone
cannot satisfy the native guard. Native compilers, SDKs and linked system
libraries must be recorded/reviewed in the later release audit.

Only Darwin arm64 runs upstream's Metal `go generate` directive. The build is
`go build -mod=readonly -trimpath -o ... ./cmd/runner`. It retains all four
upstream dispatcher engines and their cgo inputs. No CUDA/ROCm/Vulkan/MLX
shared library is bundled or qualified. Optional library loading and CPU ISA
portability still require native verification.

## Signed release artifact contract

For each `runner-OS-ARCH`, require:

- Raw executable (Windows: `runner-windows-amd64.exe`).
- `runner-OS-ARCH.gz`: level-9 gzip of those exact bytes, empty filename,
  mtime zero; deterministic for the same bytes and compression implementation.
- `runner-OS-ARCH.json`: `bashy-official-runner-v1`, exact upstream repo/tag/SHA,
  target, entrypoint, Go toolchain, cgo/workspace settings, executable/gzip
  names and SHA-256 of both forms. No Yoke-source identity substitution.
- `.inputs.json` (tracked source hashes including generated shader bytes) and
  `.deps.json` (`go list -deps -json` for that target). These are inventories,
  **not** compiled-input license clearance or an SBOM.

The complete set also requires `runner-pin.txt`, `runner-NOTICES.txt`,
`runner-sbom.cdx.json`, `runner-vulnerabilities.json`, `runner-licenses.json`.
The shared pipeline binds all bytes with `SHA256SUMS`, a Sigstore bundle for
**each file including SHA256SUMS**, and checksum-based build attestations.
For eventual release tag `T=bashy-v0.21.2-N`, the exact identity is
`https://github.com/qiangli/ollama/.github/workflows/bashy-build.yml@refs/tags/T`,
issuer `https://token.actions.githubusercontent.com`. Consumers must authenticate
that identity and issuer, then check compressed and extracted byte digests and
target/source metadata before execution. A writable cache sidecar, ad-hoc
codesign, or an unsigned digest is not release authentication. Any required
platform signing must precede packaging/hashing; never modify verified bytes.

## Explicit holds and draft integration

`no-egress-test.sh` unconditionally fails until all five native runtime gates
(health, load, streaming completion, embedding, cancellation, shutdown) and
isolated no-egress tests are implemented with reviewed model fixtures.
`audit.yml` also unconditionally fails pending Go/cgo/native dependency
vulnerability analysis, compiled-input license review, generated-input review,
SBOM and notices. There is no skip/environment override. Release depends on
both build and audit jobs. The manifest records pending admission and the
release note explicitly records the hold. Clearing it requires reviewed code
and measured evidence in the parent story, not renaming a tag.

The older `origin/s436/runner-release-draft` (`38b43315`, producer in `5427419b`)
is preserved. Its `scripts/pinned-yoke-runner.sh build` follows Yoke's fork
gitlink and is superseded by this producer. Do not cherry-pick that draft's
stale Go/Yoke pins or enable it unchanged. The manager can layer this commit
onto the draft, then replace its producer/staging manifest handling with the
signed official artifact contract above. Consumer extraction/cache validation
and Bashy early `runner` dispatch are separate assigned stories. Windows
embedding, server compatibility, native tests and release admission remain
parent gates; this recipe alone does not make embedded inference work.

## Bounded local gate

`bash builds/check.sh ollama` runs regeneration/diff, shell syntax,
shellcheck/actionlint when installed, shared gate self-tests and
`python3 builds/ollama/test_recipe.py`. The latter uses a fixture Git repo and
a fake compiler to test five target recipes, native mismatch/unsupported
rejection, pristine/source guards, gzip determinism and digest metadata.
No local runner compilation, model execution, container or full suite occurs.
