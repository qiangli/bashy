# whisper.cpp v1.9.5 Source Build & Cache Recipes

Sprint #503 / Story #2742 / Story-ID `74ed924ba244`.

This directory provides reproducible, fail-closed, platform-aware build and cache recipes for `whisper.cpp` v1.9.5 across all six native GitHub Actions runner targets:

| Runner Label | Target Platform | Compiler / Toolchain | Dynamic Linkage Policy |
| :--- | :--- | :--- | :--- |
| `ubuntu-24.04` | `linux/amd64` | GCC / Clang, CMake | CPU-only, `-DGGML_OPENMP=OFF`, `-static-libgcc -static-libstdc++`, no GPL dylibs |
| `ubuntu-24.04-arm` | `linux/arm64` | GCC / Clang, CMake | CPU-only, `-DGGML_OPENMP=OFF`, `-static-libgcc -static-libstdc++`, no GPL dylibs |
| `macos-15-intel` | `darwin/amd64` | AppleClang, CMake | CPU-only, `-DGGML_OPENMP=OFF`, `-DGGML_METAL=OFF`, system frameworks only |
| `macos-15` | `darwin/arm64` | AppleClang, CMake | CPU-only, `-DGGML_OPENMP=OFF`, `-DGGML_METAL=OFF`, system frameworks only |
| `windows-2025` | `windows/amd64` | MSVC / llvm-mingw, CMake | CPU-only, `-DGGML_OPENMP=OFF`, `-static` (MinGW) or static CRT (`/MT`), no GPL DLLs |
| `windows-11-arm` | `windows/arm64` | MSVC / llvm-mingw, CMake | CPU-only, `-DGGML_OPENMP=OFF`, static CRT, no GPL DLLs |

## Pinned Source Archive

Upstream stable release `v1.9.5` does not publish prebuilt CLI binaries (`assets_count = 0`). Standalone executables are built strictly from the pinned official upstream source tarball:

- **Source URL**: `https://github.com/ggml-org/whisper.cpp/archive/refs/tags/v1.9.5.tar.gz`
- **Pinned SHA-256**: `ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720`
- **Expected Byte Length**: `9,626,464` bytes
- **License**: MIT (Georgi Gerganov; GGML MIT)

The build recipe verifies the source archive SHA-256 and byte length **prior to unpacking**. Any hash mismatch or corrupted download fails closed immediately.

## Canonical Coordination Cache Coordinates

Finished executables are hashed (SHA-256 and byte length) and placed into the standard `bashy` coordination cache:

- **Linux**: `${XDG_CACHE_HOME:-~/.cache}/bashy/tools/whisper-cpp/v1.9.5/whisper-cli`
- **macOS**: `~/Library/Caches/bashy/tools/whisper-cpp/v1.9.5/whisper-cli`
- **Windows**: `%LOCALAPPDATA%\bashy\tools\whisper-cpp\v1.9.5\whisper-cli.exe`

Custom cache roots may be supplied via `--cache-dir` or `--dest-cli`.

## Runner Invocation

### POSIX Runners (`ubuntu-24.04`, `ubuntu-24.04-arm`, `macos-15-intel`, `macos-15`)

```bash
bashy/examples/transcribe/dependencies/build-whisper.sh --json
```

Or invoke the Python recipe directly:

```bash
python3 bashy/examples/transcribe/dependencies/build_whisper.py --json
```

### Windows Runners (`windows-2025`, `windows-11-arm`)

```powershell
pwsh -File bashy/examples/transcribe/dependencies/build-whisper.ps1 -Json
```

Or invoke via Python:

```cmd
python bashy\examples\transcribe\dependencies\build_whisper.py --json
```

## Flags & Options

- `--work-dir DIR`: Directory for source extraction and intermediate build artifacts.
- `--cache-dir DIR`: Base cache directory (defaults to platform standard).
- `--dest-cli PATH`: Explicit destination executable path.
- `--source-archive PATH`: Use a pre-cached source tarball (must match pinned SHA-256).
- `--skip-download`: Fail-closed if source archive is not already on disk.
- `--parallel N`: Parallel build jobs for CMake (default: CPU core count, capped at 8).
- `--json`: Output structured JSON audit report to stdout.
- `--report-file PATH`: Write structured JSON audit report to file.

## Supply-Chain & License Guarantees

1. **CPU-Only**: All GPU/accelerator backends (Metal, CUDA, Vulkan, OpenCL, etc.) are explicitly disabled in CMake flags.
2. **No OpenMP**: `-DGGML_OPENMP=OFF` is enforced across all platforms, eliminating runtime dependencies on `libgomp.so`, `libomp.dylib`, or `libgomp-1.dll`.
3. **No GPL Dynamic Dependencies**: Post-build dynamic library audit (`otool -L` on macOS, `ldd` on Linux, `objdump` on Windows) verifies zero GPL or copyleft shared libraries are linked.
4. **No Side-Effects**: The build recipe performs zero model downloads, zero media inference, zero FFmpeg dependencies, and no container / OCI requirements.
5. **No False Six-Target PASS**: The recipe reports actual host operating system, architecture, compiler version, and build flags. CI jobs run natively on each runner to establish independent evidence.
