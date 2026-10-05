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
| `typescript` / `ts` (default runtime) | `yoke/external/node`: Node 22.23.2 + `typescript@5.9.3` from npm | Node core MIT; its `LICENSE` lists ~40 bundled deps, all permissive (V8 BSD-3, ICU Unicode, OpenSSL Apache-2.0, zlib, …) including bundled npm under **Artistic-2.0** (permissive, non-copyleft; not one of the three licenses §1 names, so it is spelled out). TypeScript Apache-2.0 | download + exec, permissive (note npm) |
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
| `cargo`, `pyproject`, `gomod`, `package` | cargo, uv, go, npm | as the code-fence rows above (npm Artistic-2.0, permissive) | download + exec |
| `cmake` | CMake 4.x (`yoke/external/cmake`) | BSD-3-Clause | download + exec |
| `makefile` / `make` | in a dag body: bashy's pure-Go POSIX make. GNU make (Sprint 365 registered command) | GNU make **GPL-3.0** — downloaded or built from pinned source and executed, never bundled (`../../docs`: posix-provider-distribution-policy) | download/build + exec |

## `powershell` and `csharp` (Sprint 358)

One separately downloaded PowerShell 7.6.6 runtime serves both fences;
`csharp` is proposed to compile through `Add-Type` and the Roslyn 5.0 compiler
in that runtime, subject to S358.5's real-host probe. Nothing from Microsoft is
linked, embedded or vendored in bashy.

### Repository-level notice (not an archive ruling)

At tag `v7.6.6`, PowerShell/PowerShell's `LICENSE.txt` is MIT. Its
`ThirdPartyNotices.txt` declares package rows under MIT or BSD-2-Clause,
including `Microsoft.CodeAnalysis.Common 5.0.0` and
`Microsoft.CodeAnalysis.CSharp 5.0.0` under MIT. The repository notice has
SHA-256 `275443457d6f9eac61ef2367da89b1420b94f68c1cc94ed305d2591514ea88d1`.
This is repository-level evidence only; it is not used as a ruling on every
binary included in a platform archive.

### Archive-level audit (2026-10-04)

The audit downloaded all seven release archives, verified each against the
release's `hashes.sha256`, and read the root `LICENSE.txt` and
`ThirdPartyNotices.txt` from each archive. Every root license says MIT. All
seven root notices have the same content as the repository notice after CRLF
normalization (the Windows copies use CRLF). This sameness does not erase
platform-specific payloads that the common notice does not separately rule on.

| Platform | Release asset | SHA-256 | Root files read | Platform-specific finding |
|---|---|---|---|---|
| Windows x64 | `PowerShell-7.6.6-win-x64.zip` | `02fe458be20493fbdf43f61ea20610b811ee6c738ab1676c61b9cfcd1a33c860` | MIT license; common notice | Adds closed Microsoft native redistributables, including `D3DCompiler_47_cor3.dll` and `vcruntime140_cor3.dll`, plus WPF assemblies. The common notice is not a blanket license for these members. |
| Windows arm64 | `PowerShell-7.6.6-win-arm64.zip` | `bbde9dda31d148415eccb5fbe1638e6400a144187b006e5b3fd8ec2f39d781be` | MIT license; common notice | Adds `vcruntime140_cor3.dll` and WPF assemblies (but not the x64 archive's `D3DCompiler_47_cor3.dll`). These platform members remain distinct from the common notice. |
| Linux x64 (glibc) | `powershell-7.6.6-linux-x64.tar.gz` | `ddbc4a2d113bbd46d283cfedcbcd117a70caefd7673f41f2b4e0000badf103bc` | MIT license; common notice | No Windows-only native payloads. |
| Linux arm64 (glibc) | `powershell-7.6.6-linux-arm64.tar.gz` | `924829e54c983648f6f1419a2dc7f9433c861b2fb5bd57736ff096c24f133729` | MIT license; common notice | No Windows-only native payloads. |
| Linux x64 (musl) | `powershell-7.6.6-linux-musl-x64.tar.gz` | `9537c256a60c34f6bc2dd60c1c10b31a0c2ef26e96799d066be78325ab4947cc` | MIT license; common notice | No Windows-only native payloads. |
| macOS x64 | `powershell-7.6.6-osx-x64.tar.gz` | `e325ed9f666894eb39a5ea52800b602da2fb4242bbe9747ceddb39cdc66de805` | MIT license; common notice | No Windows-only native payloads. |
| macOS arm64 | `powershell-7.6.6-osx-arm64.tar.gz` | `6df833d094ebac1c1a74340d7b3437f4aaf5e03ce640484a1c4359f3ce8b3db1` | MIT license; common notice | No Windows-only native payloads. |

This archive inventory records what was actually inspected without promoting
the repository-level MIT/BSD notice into a conclusion about unlisted native
libraries. The separately downloaded runtime is still download + exec under
§2. S358.5 must record a live `Add-Type` run per required OS. S358.9 separately
owns the FROM-scratch image's libstdc++, libgcc and ICU runtime ruling.

## Not a fence, same inventory

`bashy java` / `javac` / `mvn` are advertised in the command synopsis; Eclipse
Temurin is **GPL-2.0 with Classpath Exception** (not permissive) and Maven is
Apache-2.0. Provisioning and the honest license record are Sprint 350 story
387 (`docs/todo/33099a9a1849-*`).
