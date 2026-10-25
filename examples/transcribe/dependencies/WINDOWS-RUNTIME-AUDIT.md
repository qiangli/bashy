# S503.6 Windows Whisper runtime import correction

The six-target source-build run [40201076574](https://github.com/qiangli/bashy/actions/runs/40201076574)
at Bashy `7abe1fd1` passed its build gate. Its Windows artifacts still depend on
external Visual C++ runtime DLLs. The downloaded build reports record:

| Target | Compiler | `whisper-cli.exe` SHA-256 | Unwanted imports |
| --- | --- | --- | --- |
| Windows amd64 | `C:\Program Files\LLVM\bin\clang.exe` 20.1.8 | `f842ae94d0cd86350ba3c22563316b1a9f8a3dfc17745d240f5a791546c04551` | `MSVCP140.dll`, `VCRUNTIME140.dll` |
| Windows arm64 | `C:\Program Files\LLVM\bin\clang.exe` 22.1.8 | `fef0a20eb2189cad86e40b4ef21d79f319da910b13a7d6ca361cbb096f6ddf16` | `MSVCP140.dll`, `VCRUNTIME140.dll` |

Both reports included `-DCMAKE_EXE_LINKER_FLAGS=-static`; the hosted Clang
selected an MSVC runtime despite that flag. These artifacts are valid native
build evidence, but are not an admitted standalone dependency path.

The replacement candidate uses native UCRT llvm-mingw release
[`20260616`](https://github.com/mstorsjo/llvm-mingw/releases/tag/20260616).
The release's published SHA-256 digests and byte counts are pinned in
`prepare_llvm_mingw.py`: x86_64 `b9b68a4d…` / 187,504,083 bytes and aarch64
`31259366…` / 181,877,750 bytes. The installer checks full size and digest
before extracting the build-only toolchain. The recipe selects its Clang pair
and `llvm-objdump`, links static C/C++ runtimes, and fails if any VC or dynamic
GNU/LLVM C++ runtime DLL is imported. Windows UCRT and OS DLL imports remain
permitted. The workflow also checks the PE machine type and emits a toolchain
provenance report.

The preparation step checks that `clang.exe` itself has the runner's native PE
machine type and executes it to check the default GNU target. The Whisper CMake
cache still must show `GGML_NATIVE=OFF`; all x86 extension options stay off for
the portable amd64 baseline. Native ARM64 execution and output remain a hosted
Windows gate.

The llvm-mingw assembly scripts are [ISC](https://github.com/mstorsjo/llvm-mingw/blob/20260616/LICENSE.txt);
the toolchain contains components with their own licenses, including
[LLVM](https://llvm.org/docs/DeveloperPolicy.html#license) and
[mingw-w64](https://www.mingw-w64.org/support/#licensing). The compiled binary
may statically include LLVM `compiler-rt`, `libc++`, `libc++abi`, `libunwind`,
and mingw-w64 runtime objects. Their exact linked object inventory and required
copyright/license notices remain a separate product packaging review. A PE
import scan proves only the dynamic dependency boundary; it cannot establish
full license admission. The compiled binary and its retained import report
still need a native Windows amd64 and arm64 workflow run and independent
artifact review. This candidate does not establish a native pass or settle the
separate FFmpeg packaging decision.

## Native candidate result, 2026-10-25

[Run 40207606199](https://github.com/qiangli/bashy/actions/runs/40207606199)
completed successfully on all six native runners at Bashy `2a490dbf`.
The manager downloaded every binary and report, rehashed them, and verified
the six signed workflow attestations against that exact source commit and
workflow identity. Both Windows executables have the correct PE machine type.
An independent `/usr/bin/objdump -p` import inspection matched the native
reports: each imports 15 Windows OS/UCRT DLLs and **zero** external VC or
third-party C++ runtime DLLs. Exact artifact IDs, digests, imports and
toolchain pins are in the [review ledger](../../../docs/sprint-503/evidence/windows-static-whisper-40207606199.json).

This establishes a candidate Windows dynamic-runtime boundary. Static-linked
runtime notices, signed release/install policy, external Python and the
operator-video E2E gate still require separate review and evidence.
