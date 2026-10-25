# S503.6 Linux Whisper CLI: permissive static-link experiment

**Status: source-level design only; `product_admitted=false` for Linux amd64
and arm64.** This is a candidate to replace the Ubuntu glibc/GCC link, not a
license ruling or a release artifact. No native build, inference, installation,
or container was run for this design.

## Why the existing link cannot close the product policy

The [two-target reviewed receipt](evidence/linux-shared-gcc-40216444004/review.json)
records Ubuntu glibc `Scrt1.o` and GCC `crtbeginS.o`/`crtendS.o`, plus dynamic
`libgcc_s.so.1` and `libstdc++.so.6`. The [source review](evidence/linux-glibc-startup-license-closure.md)
maps `Scrt1.o` to glibc `csu/init.c`, which lacks the file-level startup
linking exception. Removing `-static-libgcc` alone leaves the GCC startup
objects and dynamically loaded GCC libraries. Excluding `init.o` from
`Scrt1.o` by hand would also require a new, architecture-specific entry/ABI
implementation and a byte-level audit. These are reasons to try a coherent C,
C++, startup, unwind, and compiler-runtime replacement together.

## Proposed build and link graph

Use the **pinned Zig 0.15.2 distribution as a build tool**, selecting
`x86_64-linux-musl` on native `ubuntu-24.04` and `aarch64-linux-musl` on
native `ubuntu-24.04-arm`. Zig documents that its musl target builds libc
from source and links a static executable, without system libc files
([toolchain overview](https://ziglang.org/learn/overview/)). Its `zig cc`
and `zig c++` drivers provide Clang, LLD, musl, libc++, libc++abi, LLVM
libunwind, and compiler runtime components from the distribution. The
[Clang toolchain documentation](https://clang.llvm.org/docs/Toolchain.html)
explains why C++ standard library, ABI, unwinder, and compiler builtins must
all be accounted for, even when the C library changes.

Pin the two official Zig archives, with both the official download page's
SHA-256 and its minisign signature/public key, before executing a downloaded
compiler ([Zig downloads](https://ziglang.org/download/),
[machine-readable index](https://ziglang.org/download/index.json)):

| Native runner | Zig archive | Size | SHA-256 |
| --- | --- | ---: | --- |
| `ubuntu-24.04` | `zig-x86_64-linux-0.15.2.tar.xz` | 53,733,924 | `02aa270f183da276e5b5920b1dac44a63f1a49e55050ebde3aecc9eb82f93239` |
| `ubuntu-24.04-arm` | `zig-aarch64-linux-0.15.2.tar.xz` | 49,471,996 | `958ed7d1e00d0ea76590d27666efbf7a932281b3d7ba0c6b01b0ff26498f667f` |

The source remains the existing immutable whisper.cpp **v1.9.5** archive,
9,626,464 bytes, SHA-256
`ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720`
(`examples/transcribe/dependencies/build_whisper.py`). Preserve its existing
CPU-only, non-native `GGML_*` switches, including the amd64 SIMD exclusions.

The first experiment should use two small executable wrappers (`zig cc
-target <triple>` and `zig c++ -target <triple>`) as CMake's C and C++
compilers. Configure with `CMAKE_FIND_ROOT_PATH_MODE_LIBRARY=ONLY` and
`CMAKE_FIND_ROOT_PATH_MODE_INCLUDE=ONLY` where Zig's virtual sysroot permits
it, and no Linux branch `-static-libgcc`/`-static-libstdc++` flags. The
[first hosted run](evidence/linux-musl-40218138727-failure.md) showed that
Zig 0.15.2 rejects `-Wl,-Map=<path>` during CMake's compiler check. The
corrected candidate uses `CMAKE_EXE_LINKER_FLAGS=-static -Wl,--print-map
--verbose-link`. Zig's [argument parser](https://github.com/ziglang/zig/blob/0.15.2/src/main.zig)
accepts both options, and its [LLD path](https://github.com/ziglang/zig/blob/0.15.2/src/link/Lld.zig)
prints the map and exact linker argv. The probe extracts the final link map
from the build log, inventories its loaded archive members and the direct
object/archive argv, and stops if either evidence stream is absent. Save the
complete command and map, and fail if CMake finds host headers or libraries.
**The corrected flags remain a hypothesis until native CMake and link
evidence confirms their effect.** Determine whether Zig 0.15.2 supplies the
C++/unwind link automatically; do not paper over missing components with
Ubuntu `libstdc++`, `libgcc`, or glibc.

The desired ELF graph is one static `whisper-cli` composed of whisper.cpp/GGML
and Zig's musl/libc++/libc++abi/libunwind/compiler-runtime inputs. It has no
`PT_INTERP`, no `DT_NEEDED`, and no `GLIBC`, `GLIBCXX`, `GCC`, or `CXXABI`
version requirements. A static executable should run on a supported glibc
host because it uses the Linux syscall ABI, but that compatibility is an
inference to test on representative target kernels, including CPU baseline
and thread/locale behavior. It cannot be inferred from `readelf` alone.

## Source and notice review before any admission

The exact Zig **archive bytes** must be unpacked and their runtime source
and notice files retained, not inferred from the compiler's top-level MIT
license. The corresponding Zig 0.15.2 source locations are
[musl `COPYRIGHT`](https://github.com/ziglang/zig/blob/0.15.2/lib/libc/musl/COPYRIGHT),
[libc++ `LICENSE.TXT`](https://github.com/ziglang/zig/blob/0.15.2/lib/libcxx/LICENSE.TXT),
[libc++abi `LICENSE.TXT`](https://github.com/ziglang/zig/blob/0.15.2/lib/libcxxabi/LICENSE.TXT),
[libunwind `LICENSE.TXT`](https://github.com/ziglang/zig/blob/0.15.2/lib/libunwind/LICENSE.TXT),
and [Zig `LICENSE`](https://github.com/ziglang/zig/blob/0.15.2/LICENSE).
LLVM describes its Apache-2.0-with-LLVM-exceptions and dual-license framework
in its [developer policy](https://llvm.org/docs/DeveloperPolicy.html). The
[whisper.cpp v1.9.5 license](https://github.com/ggml-org/whisper.cpp/blob/v1.9.5/LICENSE)
and **every actually linked GGML and runtime source file** still require an
inventory and notice review. Some musl/LLVM subtrees have file-specific
credits or terms; the top-level labels are insufficient. Build-tool-only
code is distinguished from bytes linked into the delivered ELF by the map.

Do not assume that a `lib/compiler_rt*` input is governed by the LLVM
`compiler-rt` license: Zig contains its own compiler-runtime implementation.
Map each linked object/member to its source path and license in the pinned
distribution. Stop the experiment if the map contains an unclassified member
or a GPL/LGPL object. For the actual distribution, retain the exact notice
bytes and a manifest tying each to the archive digest and ELF digest.

## Exact hosted CI experiment and acceptance gates

1. Add a **manual, evidence-only** workflow with two independent native jobs,
   `ubuntu-24.04`/`amd64` and `ubuntu-24.04-arm`/`arm64`, each with a 60-minute
   limit and `product_admitted: false` in the report. It cannot publish or
   feed the installer. Coordinate the branch and pins with the sprint manager
   before dispatch. No runner is a container or emulated architecture.
   The first run of `.github/workflows/s503-linux-musl-runtime-probe.yml`
   performs only the source/link/notice/ELF checks and a bounded CLI help
   smoke. It makes no inference or installation claim.
2. Assert `uname -m`, `dpkg --print-architecture`, runner label, commit SHA,
   and known source/target constants. Fetch Zig 0.15.2 from its exact official
   URL. Compare size and SHA-256 above; verify its `.minisig` with the public
   key printed on Zig's official download page. Retain archive signature,
   verifier output, `zig version`, `zig env`, source archive, and all runtime
   license files. Reject mismatches before invoking Zig.
3. Fetch and verify the existing pinned whisper.cpp archive with
   `build_whisper.py`'s checks. Configure and build `whisper-cli` using only
   Zig's `cc`/`c++` wrappers, `-target` musl triple, `-static`, portable CPU
   flags, no OpenMP/GPU, and verbose final link with map. Retain CMake cache,
   compile commands, configure/build logs, driver trace, link map, and ELF.
4. Fail closed if any compile command or link input resolves to `/usr/include`,
   `/usr/lib`, `/lib`, a GCC installation, glibc `Scrt1.o`/`crti.o`/`crtn.o`,
   `crtbegin*.o`/`crtend*.o`, `libstdc++`, `libsupc++`, `libgcc*`, or an unknown
   source. Inspect **all** direct objects and loaded archive members, not just
   the `DT_NEEDED` list. An input name shared by musl and glibc must be
   classified by its full path and archive digest, not its basename.
5. Run `readelf -h -l -d --version-info`, `file`, and `nm -u` (or equivalent
   static-symbol inspection). Require the expected `EM_X86_64`/`EM_AARCH64`,
   no program interpreter, no dynamic library dependency, no GNU runtime
   version requirement, and no unexpected unresolved symbol. Retain the raw
   output and binary SHA-256. A `static-pie` result is acceptable only if it
   satisfies these exact checks.
6. After manager review of the first build artifacts, extend the native
   acceptance lane to run the already pinned tiny
   English and Spanish WAV/model fixtures with automatic language detection.
   Compare nonempty transcript and timing against the existing controlled
   baseline, run script and skill orientations plus TXT/SRT/VTT export, and
   repeat after blocking OS network egress. Record command, exit code,
   transcript/receipt hashes, duration, model/media hashes, and cleanup.
   A build-only success stays **experimental**.
7. Upload the ELF, map, logs, notices, source/archive receipts, and JSON report
   even on failure. Attest the exact ELF with the job's workflow identity and
   source commit. Independently download both job artifacts, rehash every
   retained input/output, verify attestations, compare map to notices, and
   record a review JSON. The report and review remain `product_admitted=false`
   until an authorized product decision, exact signed release bytes, installer
   pins, bare-host compatibility, and all six-target/operator-video gates pass.

This route has a clear fail condition: if Zig's native musl C++ closure brings
in an unpermitted or untraceable object, return to a separately pinned musl +
LLVM runtimes build rather than silently accepting a GCC/glibc fallback.
