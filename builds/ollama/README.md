# Official Ollama runner recipe — release blocked

> **Recovery map:** [`files/RECOVERY.md`](files/RECOVERY.md) records the whole
> local-inference + sphere-shard feature, how it was lost in 2026 while other
> work was added, how Sprint 436 restored it, and how to recover it again.

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

On Windows only, the recipe exports a required (not caller-selected)
`CGO_CXXFLAGS=-include cstdlib`: the needed `llama/.../models` package
(`llama-graph.h` via llamarunner) calls `getenv`/`atoi` without including
`<cstdlib>`, which MinGW clang in strict C++17 rejects while glibc/libc++
provide transitively. The header is force-included through toolchain flags
because editing the source would trip the pristine-tree pin gate; no upstream
byte changes.

The Darwin `no-egress` denial controls probe wrong-port loopback endpoints
only: Seatbelt expands `localhost` in the
`(remote ip "localhost:PORT")` allow rule to both 127.0.0.1 and ::1, so a
same-port ::1 probe is policy-allowed (connection refused) rather than
denied. Any gate failure prints the verdict and the captured evidence to the
step log via a fail-loud exit trap, and all evidence uploads run under
`if: always()`.

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

`no-egress-test.sh` now runs the hosted-only native protocol/network gate
specified below (Story #1920, `df0cc6a1965c`). It fails outside GitHub-hosted
runners and explicitly refuses dev-host. Hosted results are still pending; the
local harness tests are not native runtime evidence.
`audit.yml` remains unchanged and unconditionally fails pending Go/cgo/native dependency
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
`python3 builds/ollama/test_recipe.py`, `test_native_gate.py`, `test_audit.py`,
and `test_compiler_evidence.py`. The former uses a fixture Git repo and
a fake compiler to test five target recipes, native mismatch/unsupported
rejection, pristine/source guards, gzip determinism and digest metadata.
No local runner compilation, model execution, container or full suite occurs.

## Native protocol and network gate (Story 1920)

All five existing matrix entries run the same protocol exercise against the
**raw executable just built**, after checking its source/target metadata and
SHA-256. Windows resolves the actual `.exe`, never a PATH installation.
The default upstream llama.cpp engine is exercised, with CPU-only load parameters,
512 context tokens, one sequence slot and two threads. This is not a claim of
GPU inference, every model architecture or the other dispatcher engines.
Protocol fields/status values come from the exact pinned
[`llm/server.go`](https://github.com/ollama/ollama/blob/590109c8352e8d5a6206e8909b518a54a2b0a7b8/llm/server.go)
and [`runner/llamarunner/runner.go`](https://github.com/ollama/ollama/blob/590109c8352e8d5a6206e8909b518a54a2b0a7b8/runner/llamarunner/runner.go).

The CI-only fixture is **stories260K.gguf**, 1,185,376 bytes, SHA-256
`270cba1bd5109f42d03350f60406024560464db173c0e387d91f0426d3bd256d`.
`files/fixture.json` pins the immutable ggml-org conversion revision and links
the original [MIT model card](https://huggingface.co/karpathy/tinyllamas/blob/0bd21da7698eaf29a0d7de3992de8a46ef624add/README.md).
The MIT notice is retained in `fixture-LICENSE.txt`. Download happens **before**
isolation; byte count, magic and exact hash are checked again inside the gate.
No model registry/pull API or model download can substitute for this fixture.
The fixture itself is not a release asset; its provenance and license are.

Required observations, with bounded waits and response sizes:

1. Exclusive preflight bind on `127.0.0.1:18997`, launched health status 2,
   and OS-reported listening-socket ownership by the launched runner PID.
   Wildcard binds, another process or an occupied port fail; no port retry.
2. `/load` commit (`Operation=2`) returning `Success=true`, then ready health
   status 0. These establish that the executable loaded the exact GGUF.
3. Chunked `/completion` with nonempty content frames, a terminal done frame,
   and positive prompt/generated-token counts. Evidence retains the frames.
4. `/embedding` with positive evaluated-token count and a nonzero, finite
   numeric vector. Evidence retains the vector.
5. A streaming request with unlimited prediction and right-recursive grammar
   `root ::= "a" root` (no terminal/EOS production). After a nonterminal content
   frame, shut down its TCP connection. A new completion must finish within
   15 seconds on the **single** slot, proving the canceled work released it.
6. Explicit process termination and wait, with no listener remaining. Unix
   requires SIGTERM status and no process-group descendants; Windows requires
   the guard to terminate/reap its job before removing the WFP policy. Windows
   is a controlled forced-shutdown proof, not a graceful signal-handler claim.
   The gate ends the runner with process termination (TerminateProcess exit
   marker 1): the pinned upstream reads no stdin stop command and its own
   server ends runners with Process.Kill, so no graceful exit exists to wait
   for — only termination, reaping, and proof that no listener remains.

| Native hosts | Enforcement and evidence |
|---|---|
| Linux amd64/arm64 | New network namespace with only loopback; unprivileged exercise and entire child tree under strace. The shared verdict rejects connect/send destinations outside the configured loopback port, **including denied attempts**. Missing/nonempty trace checks and raw syscall evidence are mandatory. |
| Darwin amd64/arm64 | `sandbox-exec` denies outbound networking except IPv4 loopback:18997, inherited by the exercise and child tree. TCP and UDP controls require EPERM/EACCES for external IPv4, wrong loopback port, and IPv6. Timeout/refused/unreachable does not count. Successful protocol calls are the allowed-path control. |
| Windows amd64 | Elevated hosted runner required. A small MinGW-built guard installs dynamic WFP ALE_AUTH_CONNECT filters keyed to the exact application path before starting it. IPv6, non-127.0.0.1 IPv4 and other remote ports are blocked. Six TCP/UDP denial controls require WSAEACCES; a configured-loopback control must connect. The runner starts suspended, enters a kill-on-close job limited to one process, then resumes. Dynamic policy survives until the runner is reaped. Missing WFP/admin/compiler support fails. |

Darwin/Windows prove **kernel-enforced absence of disallowed outbound traffic**
during this exercise; they do not claim a complete log of attempted connections.
Windows uses WFP directly because ordinary Windows Firewall rules do not provide
this loopback-port confinement. The layer semantics are documented in
[Microsoft's ALE layer reference](https://learn.microsoft.com/en-us/windows/win32/fwp/ale-layers).
The helper is test infrastructure, not part of the runner artifact.

Each platform uploads `runner-OS-ARCH-native.json`, `-native.log` and `-egress.txt`
with `if: always()` and missing-files treated as error. A protocol pass alone is
`PROTOCOL_PASS`; only the shell's successful network verdict can finalize `PASS`.
JSON includes target, OS, source SHA, executable and fixture hashes, each protocol
observation and the hash of network evidence. The same files are required release
assets and enter the existing all-file checksums/signing flow. No signing can
proceed while any build/native lane or the separate audit job fails.

### Worker verification and remaining hosted work

On dev-host: the bounded recipe/harness gate and generated-fork diff are the only
execution checks. The 1.2 MB fixture was downloaded and its exact hash and GGUF
metadata verified by reading bytes; no native build, model load or inference ran.
The conductor must review/install the generated fork and run all five hosted
lanes. A missing platform PASS is still a release blocker. The Windows helper
has not been compiled or executed locally; its first native verification is that
hosted lane. This worker does not claim any of the five runtime lanes passed.


## Compiler evidence (Story 1927)

Each compiler invocation records its cwd, original/executed argv, exit status,
and unique dependency file. Before returning to Go, the wrapper resolves relative
inputs in that cwd and saves their SHA-256 and bytes in a content-addressed store.
Separate invocation files and atomic blob writes support parallel compiles. The
collector verifies blob hashes and dependency closure after Go deletes its work
files; it never reopens vanished temporary inputs. Repository C/C++/headers and
Metal remain in the exact-hash native license review inventory.

The builder allocates private Go temp/cache directories. Only Go's cgo probe
names and known generated cgo files under numbered Go work-package directories,
and runtime/cgo under the pinned GOROOT, receive Go provenance. Symlinks are
resolved before classification; arbitrary temp files and module-cache native
inputs fail closed. Trusted Go inputs still carry path, SHA-256 and saved content.
This classification is build provenance, not a new third-party license clearance.

Windows setup discovers GCC/G++ on PATH and under installed MSYS2 environments,
requiring matching versions/installations, the x86_64-w64-mingw32 target and
GCC/MinGW/64-bit/C++ macros. It records and exports exact selected paths; the
build rechecks the pair and records executable hashes and versions. The Windows
Go compiler command explicitly invokes Python because CreateProcess does not
interpret shebangs. No compiler is installed or silently substituted.

The compiler-evidence artifact uses `always()` independently of no-egress and
audit outcomes. Unsigned release inputs retain normal success conditions, and
signing still requires pin, all native builds, and audit. Diagnostic bytes are
not a release verdict. Hosted confirmation and exact native license review remain
required; the bounded local fixtures do not execute native compilers or runners.

## CUDA target (Sprint #451 / Story #2487)

`runner-linux-amd64-cuda` is a **separate** matrix target (ubuntu-24.04). The
Go runner alone is not CUDA: `build-runner.sh` additionally builds
`libggml-cuda.so` from the same pristine pinned tree with upstream's
`CUDA 12` CMake preset, copies it (never a symlink) to
`runner-linux-amd64-cuda-libggml-cuda.so` as its own signed release asset, and
`package-runner.py` records it in the manifest as `backend_artifacts`
(name/sha256/size) plus `backend_identity` (source commit == pin, nvcc version
and sha256, preset). CPU/Metal targets are unchanged.

**Blockers (release stays held):** (1) the hosted runner has no CUDA toolkit;
installing NVIDIA's toolkit/runtime is outside the permissive-only policy and
needs a license decision, so the build fails closed without `nvcc`; (2)
`audit-runner.py final` fails while any CUDA manifest is not `ADMITTED` (no
linkage/license/vulnerability/SBOM review of the CUDA bytes exists; nvcc
compiles are not captured by compiler evidence); (3) no GPU on hosted runners,
so the native gate exercises only CPU fallback and makes no CUDA inference
claim. Not compiled or run locally.
