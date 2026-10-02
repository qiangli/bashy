# Bash# fence toolchains — what runs them and under which license

Status: inventory of record, 2026-10-02. Seed of the Sprint 350
runtime-acquisition inventory for the fence languages; the policy it applies
is `licensing-supply-chain-policy.md` (§2: download + exec is not bundling;
prefer permissive anyway; record the source).

Every claim below was read from the pinned archive's own license file (in the
binmgr cache) or the upstream project's license file — never from a
repository badge. Where an archive bundles parts under other terms, the
bundled parts are named; a one-word label is never the whole story.

## The mechanism

- Registry: `sh/polyglot/languages.go` (`RegisterLanguage`). Parser:
  `sh/syntax/bashpp_source_block.go`. Dispatch: `sh/interp/bashpp_polyglot.go`.
- Tool resolution under bashy: `internal/agentos/toolchains.go`
  (`islandToolchains`). A fence **never resolves its tool from PATH**; each
  foreign-language tool is a pinned, digest-verified download executed as a
  **separate process**. Only a `BASHPP_*` variable overrides it.
- The `bash` and `sh` fences are different in kind: they are bashy's own
  implementation — a fresh in-process `interp.Runner` on the pure-Go Bash 5.3
  engine (`sh/interp/shell_polyglot.go`). No GNU Bash, no host `/bin/sh`, no
  download.
- The engine itself (`sh`, `bashsharp`) links **no** third-party language
  runtime (no goja/starlark/yaegi/lua/wasm) and no GNU Bash code (clean-room
  rule in `../../sh/THIRD_PARTY_LICENSES.md`). The worker protocols are our
  code. The compile-in layer of the fence feature is therefore BSD/MIT only.

## Code fences

| Fence | Provisioner · pin | License of the downloaded artifact (as read) | Class |
|---|---|---|---|
| `bash`, `sh` | in-process, bashy's own engine | first-party (sh fork BSD-3-Clause) | compiled in, first-party |
| `python` / `py` | `yoke/external/python`: uv → python-build-standalone CPython (uv 0.5.11, Python 3.13 line) | uv Apache-2.0 OR MIT. CPython PSF-2.0. python-build-standalone links **libedit, not GNU readline**, and disables `_gdbm`, specifically to avoid GPL (builds since 2023); the per-build component list ships in the archive's `PYTHON.json` | download + exec, permissive |
| `typescript` / `ts` (default runtime) | `yoke/external/node`: Node 22.23.2 + `typescript@5.9.3` from npm | Node core MIT; its `LICENSE` lists ~40 bundled deps, all permissive (V8 BSD-3, ICU Unicode, OpenSSL Apache-2.0, zlib, …) **except bundled npm = Artistic-2.0** (OSI-approved, outside the MIT/BSD/Apache trio). TypeScript Apache-2.0 | download + exec, permissive (note npm) |
| `typescript` with `BASHPP_TYPESCRIPT_RUNTIME=bun` or a bun lockfile | `yoke/external/bun`: Bun 1.4.2 | Bun's own code MIT, but the shipped binary **statically links JavaScriptCore/WebKit (LGPL-2) and tinycc (LGPL-2.1)**; zstd is dual BSD/GPL-2 (upstream `LICENSE.md`). The release zip carries no license file | download + exec; **not permissive as a program**; never the default |
| `rust` / `rs` | `yoke/external/rust`: rustup-init → `stable`; on Windows `stable-x86_64-pc-windows-gnu` with `cc-linker` = zig cc | Rust Apache-2.0 OR MIT; LLVM Apache-2.0-with-LLVM-exception. Worker crates serde, serde_json, base64 (MIT OR Apache-2.0) fetched by cargo on the user's machine | download + exec, permissive |
| `c`, `cpp` / `cxx` | `yoke/external/zigcc`: Zig 0.16.0 `zig cc` / `zig c++` | Archive `LICENSE`: MIT. Bundled under `lib/`: libc++, libc++abi, libunwind Apache-2.0-with-LLVM-exception; musl MIT; wasi-libc Apache/MIT; **mingw-w64 ZPL-2.1** (parts public domain / BSD / LGPL per its `COPYING`); **glibc headers, csu and abilists LGPL-2.1+** (`lib/libc/glibc/LICENSES`) | download + exec, permissive compiler; the libc trees are compile-time inputs to workers built on the user's machine, never distributed by bashy |
| `c`, `cpp` on Windows via `sh/polyglot/msvc.go` | host MSVC + Windows SDK headers found through vswhere | proprietary, host-supplied | never downloaded or provisioned by bashy |
| `c`, `cpp` on Windows via `bashy clang` (`yoke/external/clang`) | llvm-mingw (mstorsjo) | llvm-mingw ISC; built toolchain "primarily LLVM Apache-2 with exceptions"; mingw-w64 CRT ZPL / public domain / BSD | download + exec, permissive |
| `go` | `yoke/external/gotoolchain`: Go 1.27.1 (also the lowering SDK, `sh/lower/go_sdk.go`) | BSD-3-Clause | download + exec, permissive |

## Text and manifest fences

The tool behind each is a CLI the fence drives with a verb table.

| Fence | Tool | License | Class |
|---|---|---|---|
| `dockerfile` | podman (bashy-built from source) | Apache-2.0 | download + exec |
| `tf` / `tofu` / `hcl` | OpenTofu (`yoke/external/registry/tofu.go`) | **MPL-2.0** (recorded in the registry entry) | download + exec, weak copyleft |
| `k8s` / `kube`, `helm` | kubectl, helm | Apache-2.0 | download + exec |
| `skill`, `dag` | bashy itself | first-party | in-process |
| `cargo`, `pyproject`, `gomod`, `package` | cargo, uv, go, npm | as the code-fence rows above (npm Artistic-2.0) | download + exec |
| `cmake` | CMake 4.x (`yoke/external/cmake`) | BSD-3-Clause | download + exec |
| `makefile` / `make` | in a dag body: bashy's pure-Go POSIX make. GNU make (Sprint 365 registered command) | GNU make **GPL-3.0** — downloaded or built from pinned source and executed, never bundled (`../../docs`: posix-provider-distribution-policy) | download/build + exec |

## Planned: `powershell` and `csharp` (Sprint 358, planning only)

Design: one pinned download, PowerShell 7.6.6 (assets for win-x64/arm64,
linux-x64/arm64/musl-x64, osx-x64/arm64, with a published `hashes.sha256`),
run as a persistent worker; C# compiles through `Add-Type` with the Roslyn 5.0
compiler (MIT) inside the same archive. Nothing from Microsoft is linked,
embedded or vendored. Posture, verified at the repository level:

- PowerShell repository license MIT; its `ThirdPartyNotices.txt` names only
  MIT / BSD-2 packages (Markdig, Newtonsoft.Json, Microsoft.CodeAnalysis.CSharp
  5.0.0, …).
- Archive-level caveats the sprint already records: the Windows zip carries a
  few closed Microsoft natives (`D3DCompiler_47_cor3.dll`,
  `vcruntime140_cor3.dll`, WPF) listed in its own notices; the .NET **SDK**
  Windows zip is under the non-OSI ".NET Library" terms and stays an explicit
  opt-in for the NuGet tier only, never called permissive.
- Still to verify in S358.1 / S358.5: `LICENSE.txt` and `ThirdPartyNotices.txt`
  inside all seven pinned archives (only win-x64 has been read), and a live
  `Add-Type` run per OS.
- S358.9 (FROM-scratch image) needs libstdc++ and libgcc (GPL with the runtime
  library exception) and ICU as separately downloaded files; that ruling is a
  story deliverable.

Verdict: consistent with §2 — permissive upstream, download + exec, one
non-OSI piece behind an opt-in.

## Not a fence, same inventory

`bashy java` / `javac` / `mvn` are advertised in the command synopsis; Eclipse
Temurin is **GPL-2.0 with Classpath Exception** (not permissive) and Maven is
Apache-2.0. Provisioning and the honest license record are Sprint 350 story
387 (`docs/todo/33099a9a1849-*`).
