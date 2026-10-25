# whisper.cpp v1.9.5 Source Build & Cache Recipes

Sprint #503 / Story #2742 / Story-ID `74ed924ba244`.

This directory provides reproducible, fail-closed, platform-aware build and cache recipes for `whisper.cpp` v1.9.5 across all six native GitHub Actions runner targets:

| Runner Label | Target Platform | Compiler / Toolchain | Dynamic Linkage Policy |
| :--- | :--- | :--- | :--- |
| `ubuntu-24.04` | `linux/amd64` | GCC / Clang, CMake | CPU-only, `-DGGML_OPENMP=OFF`, `-static-libgcc -static-libstdc++`, no GPL dylibs |
| `ubuntu-24.04-arm` | `linux/arm64` | GCC / Clang, CMake | CPU-only, `-DGGML_OPENMP=OFF`, `-static-libgcc -static-libstdc++`, no GPL dylibs |
| `macos-15-intel` | `darwin/amd64` | AppleClang, CMake | CPU-only, `-DGGML_OPENMP=OFF`, `-DGGML_METAL=OFF`, system frameworks only |
| `macos-15` | `darwin/arm64` | AppleClang, CMake | CPU-only, `-DGGML_OPENMP=OFF`, `-DGGML_METAL=OFF`, system frameworks only |
| `windows-2025` | `windows/amd64` | Clang / MSVC, CMake | CPU-only, `-DGGML_OPENMP=OFF`; CI binary imports external Microsoft VC DLLs |
| `windows-11-arm` | `windows/arm64` | Clang / MSVC, CMake | CPU-only, `-DGGML_OPENMP=OFF`; CI binary imports external Microsoft VC DLLs |

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

## Explicit operator source setup

`binmgr.EnsureWhisperCpp` currently has no committed stable v1.9.5 CLI release
pin and fails closed. On a native host with an operator-provided Python interpreter and
compiler toolchain, the operator can explicitly build from the same pinned
stable source using:

```sh
python3 examples/transcribe/dependencies/setup_whisper.py --work-dir /absolute/work/dir
```

`setup_whisper.py` calls `build_whisper.py`'s source recipe, checks its CPU-only
build report and rehashes the binary in the canonical coordination cache. It
atomically writes `source-build-receipt.json` beside `whisper-cli` with the
exact executable path and SHA-256. The JSON output also contains a
`registration` object with `WHISPER_PATH` and `WHISPER_SHA256`; supply that pair
to `examples/transcribe/adapters/register.sh`. The seed inference adapter
rehashes the configured executable before every invocation. A preverified
archive from `binmgr.EnsureWhisperCppSource` may be supplied with
`--source-archive PATH --skip-download`; the recipe still verifies its pinned
length and SHA-256 before extraction.

On a later session, run:

```sh
python3 examples/transcribe/dependencies/setup_whisper.py --verify-existing
```

This checks that the native target, v1.9.5 source pin, local
receipt, cached executable size and SHA-256 still agree. It prints the same
registration pair without downloading or rebuilding. On Windows, invoke this
with the selected external `python.exe` path in place of `python3`. This is a
local cache identity check; the receipt itself is not signed or a substitute
for a release artifact and license review.

This is an explicit source-build setup command. Ordinary transcription,
adapter registration and Bashy startup do not invoke it or compile a tool.
The receipt is a local byte-identity record, not a signed release asset or
license admission. A managed release still needs committed pins and signature
verification in `EnsureWhisperCpp`. Product installation of external Python,
the Windows VC runtime, and the compiler toolchain remains a separate gate.

## External Python prerequisite for YouTube acquisition

`source_youtube.py` requires `BASHY_TRANSCRIBE_PYTHON` to name the Python
interpreter running that adapter. Before it invokes the reviewed yt-dlp zipapp,
it checks the interpreter's exact patch, executable SHA-256, process and native
OS/architecture against all six pins in `external-python-runtime.json`. Missing
or changed pins, a missing path, and an interpreter different from the running
one fail closed. On Windows it additionally runs the existing
`verify_windows_external_runtime.py` check for the separately installed native
System32 `MSVCP140.dll` and `VCRUNTIME140.dll` versions and hashes. Windows
preparation retains its earlier pre-execution Python hash check.

The policy records the CI-observed external Python 3.12.15 Linux and 3.12.10
macOS/Windows executable bytes and their independently checked archive
provenance. It is a prerequisite check, not an installer or permission to
redistribute Python or Microsoft DLLs. The Linux `actions/python-versions`
archive setup script is specific to the CI tool cache and is not an admitted
product install path. Python.org's macOS and Windows installers have archive
provenance but no six-target operator-host installation proof. `elephant` had no
installed Python and older VC runtime bytes at the read-only probe. Because the
adapter itself runs under Python, this check gates its child yt-dlp invocation;
an OS-level launcher would be required to verify Python before starting the
adapter. Exact pins require review when an external installation changes.

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
3. **Dynamic Import Check**: Post-build inspection (`otool -L` on macOS, `ldd` on Linux, `objdump` on Windows) rejects missing inspection tools, unresolved imports and named OpenMP libraries. This name-based check is not a full transitive license audit; Windows CI binaries import external Microsoft VC runtime DLLs.
4. **No Side-Effects**: The build recipe performs zero model downloads, zero media inference, zero FFmpeg dependencies, and no container / OCI requirements.
5. **No False Six-Target PASS**: The recipe reports actual host operating system, architecture, compiler version, and build flags. CI jobs run natively on each runner to establish independent evidence.
