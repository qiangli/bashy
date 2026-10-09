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

## `fsharp` source files (`.fs`, `.fsx`; Sprint 379)

F# source files run through `dotnet fsi`; the island resolver's `dotnet` row
(`internal/agentos/dotnet.go`) provisions the .NET SDK 10.0.401 (release
10.0.12, 2026-09-08) with `binmgr`: cache-first, the archive pinned per
platform by the SHA-512 in Microsoft's release metadata (committed in the
source, so the anchor ships with bashy), refused on mismatch. Download + exec
only; nothing from the SDK is linked into bashy. `BASHPP_DOTNET` names an
installed dotnet instead and skips the download.

All six pins were checked against the downloaded bytes (2026-10-09), and the
root `LICENSE.txt` of each archive was read:

| Platform | Asset | Root `LICENSE.txt` | Class |
|---|---|---|---|
| Linux x64 / arm64 (glibc) | `dotnet-sdk-10.0.401-linux-{x64,arm64}.tar.gz` | MIT (.NET Foundation) | download + exec, permissive |
| macOS x64 / arm64 | `dotnet-sdk-10.0.401-osx-{x64,arm64}.tar.gz` | MIT; `LICENSE.txt` sha256 `cfc21f5e…7310`, `ThirdPartyNotices.txt` sha256 `2dc8f8c5…cc7a`, no GPL/LGPL entry | download + exec, permissive |
| Windows x64 / arm64 | `dotnet-sdk-10.0.401-win-{x64,arm64}.zip` | **Microsoft .NET Library licence terms** (not MIT: use to "design, develop and test your programs"); the zip also carries closed Microsoft redistributables such as `vcruntime140_cor3.dll` under `shared/Microsoft.WindowsDesktop.App` | download + exec, **proprietary**: opt-in only |

Because the Windows terms are not permissive, the Windows download is refused
unless `BASHY_DOTNET_ACCEPT_LICENSE=1` is set (the message names the licence and
`BASHPP_DOTNET` as the alternative). A cached SDK is used without the opt-in.
Linux musl is not provisioned (no `linux-musl` row); use `BASHPP_DOTNET`.

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
On Linux, the managed child defaults to .NET invariant globalization so the
runtime starts on minimal systems without ICU, including the FROM-scratch
image. A host with ICU can explicitly set
`DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=0` to use culture data. The clean
Ubuntu WSL2 probe exposed the missing-ICU failure and passed after this
setting; the separate Linux host gate remains open.

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
§2. S358.5 must record a live `Add-Type` run per required OS. The
FROM-scratch image's libstdc++, libgcc and ICU ruling is the next section.

### FROM-scratch image runtime libraries (S358.9) — licence ruling

Recorded 2026-10-04, before any fetch code. One ruling for this story and for
Sprint 359's glibc image (S359.12), which needs the same C++ and GCC runtime.

**What the image needs, as read from the binaries.** The bashy image has no
libc. Every native part of .NET 10 and PowerShell (`pwsh`, `libcoreclr.so`,
`libclrjit.so`, `libhostfxr.so`, `libhostpolicy.so`, `libpsl-native.so`)
has `DT_NEEDED` entries for `libstdc++.so.6` and `libgcc_s.so.1` beside musl,
and no `RUNPATH`. Together they import 1,632 undefined symbols, including
the GNU libstdc++ ABI itself: `std::__cxx11::basic_string`,
`basic_stringstream`, `std::thread`, `std::condition_variable` and
`_Unwind_Resume@GCC_3.0`. LLVM's libc++ (Apache-2.0 WITH LLVM-exception)
uses the `std::__1` ABI and cannot satisfy those imports. **There is no
permissively licensed drop-in for this C++ runtime.** Building one from
permissive source (policy §3) would mean rebuilding .NET against libc++,
which is a fork of Microsoft's runtime and out of scope. OpenSSL is loaded
with `dlopen` only when the runtime uses crypto or TLS. ICU is not
provisioned. The image runs .NET in invariant globalization
(`DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1`).

**Ruling.** libstdc++ and libgcc_s are GPL-3.0-or-later WITH
GCC-exception-3.1 (the GCC Runtime Library Exception). Alpine labels the gcc
aport `GPL-2.0-or-later AND LGPL-2.1-or-later`. That label covers the whole
gcc source package, so the upstream terms above are the ones that apply to
these two libraries. They **fit the runtime-download policy (§2)**, on these
conditions, all of which hold in the code:

1. bashy never links, embeds or vendors them. The bashy binary is static,
   `CGO_ENABLED=0`, and has no `DT_NEEDED` at all. The libraries are loaded
   only into the separately executed `pwsh`/`dotnet` process, which is a
   Microsoft MIT program and not bashy.
2. bashy never ships them. No release asset, no repository file and no image
   bashy publishes contains them. bashy fetches them on the user's machine at
   first use from Alpine's own pinned, sha256-verified packages. It installs
   them as separate files in a private cache directory (never `/lib`), which
   only the PowerShell child process sees through `LD_LIBRARY_PATH`.
3. `bashy self image --with pwsh` builds the preloaded variant locally,
   through the user's own engine. The bytes come from Alpine during that
   build, the same download and exec on the user's side. **If the project
   ever publishes a prebuilt `--with pwsh` image (or the S359.12 glibc
   image), that is conveying GPL object code.** Publishing it then requires
   the GPL-3.0 source offer for gcc (Alpine's `aports` plus the gcc 15.2.0
   tarball), alongside the image and its SBOM. Treat that as an operator
   decision, never a CI default.
4. The Runtime Library Exception only affects programs compiled with GCC
   against these libraries. It has no bearing on bashy, which does neither.
   It is cited only because it is the licence the files carry.

musl-obstack (GPL-2.0-or-later, no exception) is a dependency of Alpine's
glibc-compatibility shim `gcompat`. bashy does **not** use `gcompat`: the
arm64 route takes a source-built musl `libpsl-native` from Alpine instead.
No GPL-without-exception file is fetched.

**Pieces fetched only on a Linux host without glibc.** Every piece is
sha256-pinned per architecture and verified before use. The Alpine pins are
in `yoke/pkg/muslrt` (one provisioning path, shared with Python's loader and
reusable by S359.12), and the PowerShell and .NET pins are in
`yoke/external/pwsh`:

| Piece | x64 (amd64) | arm64 | Source and version | Licence (as read) | Class |
|---|---|---|---|---|---|
| musl loader `ld-musl-<arch>.so.1` | yes | yes | Alpine v3.24 `musl-1.2.6-r2` (the Python row's pin, shared) | MIT | download + exec, permissive |
| PowerShell, self-contained musl | `powershell-7.6.6-linux-musl-x64.tar.gz` | — | upstream release (row above) | MIT; common notice | download + exec |
| PowerShell, framework-dependent | — | `powershell-7.6.6-linux-x64-musl-noopt-fxdependent.tar.gz`, sha256 `29a3d89b5d54f3aa67decaf64bd9cbf72cea469aa9e69330e5dc2c5ffdb37f38`. IL only, not ReadyToRun, so architecture-neutral. Its x64 apphost is not used | upstream release, `hashes.sha256` | `LICENSE.txt` MIT (sha256 `7c77a44a…f744`); `ThirdPartyNotices.txt` is the common notice (sha256 `27544345…88d1`) | download + exec |
| .NET runtime 10.0.12 (`dotnet` muxer, hostfxr, Microsoft.NETCore.App) | inside the self-contained archive | `dotnet-runtime-10.0.12-linux-musl-arm64.tar.gz`, sha256 `8ff79d85ec4d4caa3b90bc6cfcce9bd28c3336cb245db3d6f9203c3fd77a2d4b` (also matches Microsoft's published sha512 `8f369a9f…8744`) | `builds.dotnet.microsoft.com`, 10.0.12 (2026-09-08), the version the x64 bundle carries | `LICENSE.txt` MIT (.NET Foundation, sha256 `cfc21f5e…7310`); `ThirdPartyNotices.txt` 1,451 lines, all permissive, no GPL/LGPL entry (sha256 `2dc8f8c5…cc7a`) | download + exec, permissive |
| `libpsl-native.so` (PowerShell's native shim) | inside the self-contained archive | Alpine v3.24 community `libpsl-native-7.4.0-r2`. Microsoft publishes no musl-arm64 build. This is a musl source build of PowerShell-Native, with the same 33 exported functions as the copy in 7.6.6 (checked symbol by symbol) | Alpine | MIT (PowerShell-Native) | download + exec, permissive |
| `libstdc++.so.6` (6.0.34) | yes | yes | Alpine v3.24 main `libstdc++-15.2.0-r5` | GPL-3.0-or-later WITH GCC-exception-3.1 (ruling above) | download + exec, **not permissive**, ruled acceptable |
| `libgcc_s.so.1` | yes | yes | Alpine v3.24 main `libgcc-15.2.0-r5` | GPL-3.0-or-later WITH GCC-exception-3.1 (ruling above) | download + exec, **not permissive**, ruled acceptable |
| `libssl.so.3`, `libcrypto.so.3` | yes | yes | Alpine v3.24 main `libssl3-3.5.9-r0`, `libcrypto3-3.5.9-r0`. Only the two libraries are installed; engines and the legacy provider are not | Apache-2.0 | download + exec, permissive |
| ICU | no | no | not provisioned; invariant globalization | — | — |

A glibc host or macOS/Windows host fetches none of these. Its rows are the
archive table above.

## Not a fence, same inventory

`bashy java` / `javac` / `mvn` are advertised in the command synopsis; Eclipse
Temurin is **GPL-2.0 with Classpath Exception** (not permissive) and Maven is
Apache-2.0. Provisioning and the honest license record are Sprint 350 story
387 (`docs/todo/33099a9a1849-*`).
