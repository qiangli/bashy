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

The Linux recipe retains a bounded final linker map and archive/object
inventory in native CI. It distinguishes `libstdc++` and `libgcc` objects
selected into the binary from dynamic imports; exact notices and exception
questions are tracked in [LINUX-RUNTIME-AUDIT.md](LINUX-RUNTIME-AUDIT.md).
This evidence does not admit a product dependency.

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

For operator-host preparation, run
`bashy examples/transcribe/dependencies/prepare_python_host.sh --plan` on the
native target. The [six-target procedure and remaining admission gates](../../../docs/sprint-503/python-operator-install-plan.md)
cover the pinned Python.org macOS/Windows installer artifacts, native signature
checks, Windows System32 VC prerequisite, and the existing Linux signed-source
route. `--verify-artifact` checks the pinned package without installation.
The separate `operator-python-admission.json` currently blocks `--install` and
`--verify-existing` on every target until independently measured and approved
installer and installed-interpreter pins are recorded. The CI executable hash
does not establish the post-install Python.org interpreter hash. No mode uses
an `actions/setup-python` tool-cache archive as a product installer.
On Windows, an admitted `--verify-existing` would first check the installed
`python.exe` hash, native System32 VC files and app-local `VCRUNTIME140.dll`
hash before starting Python, then run a hash-pinned isolated
probe under that selected interpreter. The probe requires the actual loaded
`MSVCP140.dll` from System32 and `VCRUNTIME140.dll` from the Python install
directory to match the exact amd64 or arm64 hashes in
`windows-external-runtime.json`. A System32 prerequisite check alone does not
establish the loaded module. All six targets remain unadmitted.

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
product install path. The explicit Linux source build below passed native
source-build and offline byte-recheck gates on amd64 and arm64 in
[run 40207782322](https://github.com/qiangli/bashy/actions/runs/40207782322).
The [retained evidence](../../../docs/sprint-503/evidence/linux-python-source-40207782322/review.json)
records 3,356 installed files, 71 extension modules, and exact system-library
closure for each target. A later review found that both builds lacked the
`_bz2` and `_lzma` native modules; the previous presence-only check of their
Python wrappers did not prove working archive codecs. The functional codec
gate must pass on a new native run before those codecs are claimed. The
receipt status remains `local_source_build_unadmitted`:
library licenses, operator-host installation, and product distribution still
need review. Python.org's macOS and Windows installers have archive
provenance but no six-target operator-host installation proof. `elephant` had no
installed Python and older VC runtime bytes at the read-only probe. Because the
adapter itself runs under Python, its check gates the child yt-dlp invocation.
For a check before Python starts, invoke the adapter through the native Bashy
shell launcher:

```sh
BASHY_TRANSCRIBE_PYTHON=/absolute/path/to/python3.12 \
  bashy examples/transcribe/launch_python.sh REQUEST.json
```

The launcher checks the committed policy's exact SHA-256, selects a host
OS/architecture entry, resolves and hashes the chosen executable, then starts
Python. The YouTube pipeline gate registers the launcher as its source command.
With `BASHY_TRANSCRIBE_PYTHON_SOURCE_PREFIX` set on Linux, it checks the
signed-source provenance fields and executable hash in the local build receipt;
the adapter then rechecks the complete installed tree and dynamic libraries.
This is a local drift check. A writable launcher and receipt, or a change
between hashing and process start, is outside its integrity guarantee. Exact
pins require review when an external installation changes. Signed product
delivery, operator-host installation, complete library licenses, and six-target
native launcher execution remain separate acceptance gates.

### Explicit Linux Python.org source setup (amd64 and arm64)

[Python.org's 3.12.15 release](https://www.python.org/downloads/release/python-31215/)
is source only. On the **native Linux host**, the operator may supply these
three downloaded inputs to `setup_python_source.py`:

- `Python-3.12.15.tar.xz` from `https://www.python.org/ftp/python/3.12.15/Python-3.12.15.tar.xz` (20,829,824 bytes; SHA-256 `c2c4321961fab0fb999d66e0cecf521c2ab3994c7992873ea99e306c1094fd5a`)
- Its detached `Python-3.12.15.tar.xz.asc` signature from the same directory
  (SHA-256 `e80124b35dabf82264f254dbb291697df96768f6e4ee0e43d7e8f110a9ea50b6`)
- Thomas Wouters's public key linked by [Python.org's OpenPGP page](https://www.python.org/downloads/metadata/pgp/) (pinned fingerprint `7169605F62C751356D054A26A821E680E5FA6305`)

The command verifies the archive length, hash and detached signature with an
isolated GPG home before unpacking or building. It first copies all three
operator inputs into a new private work directory and verifies those staged
bytes; caller-path changes cannot alter the archive used for extraction. The
work directory and install
prefix must be separate, absent paths beneath the operator's home. It uses no
container or system install. It requires `gpg`, `cc`, `make`, `readelf`, `ldd`,
the CPython development headers and libraries, and an OpenSSL build suitable
for Python's `_ssl` and `_hashlib` modules. The native compiler/OpenSSL choices
remain host-specific and require review.

```sh
python3 examples/transcribe/dependencies/setup_python_source.py \
  --source-archive "$HOME/downloads/Python-3.12.15.tar.xz" \
  --signature "$HOME/downloads/Python-3.12.15.tar.xz.asc" \
  --release-key "$HOME/downloads/thomas-wouters.gpg" \
  --work-dir "$HOME/.cache/bashy/python-3.12.15-build" \
  --prefix "$HOME/.local/bashy/python-3.12.15"
```

The recipe disables `readline`, `_dbm` and `_gdbm` in `Modules/Setup.local`
and passes `--without-readline` with fatal unknown-option checking. It removes
the `ensurepip` bundled pip wheel directory and the standard-library test tree
before inventory; the pinned source also contains test-only wheels. It rejects
those builtins/extensions after installation and rejects loaded libraries with readline, gdbm or Berkeley DB
names. It also requires SSL, hashing and archive modules used by yt-dlp.
Its local `source-build-receipt.json` inventories each installed file and
extension module by SHA-256, the executable's ELF architecture, each loaded
system library path/hash, OpenSSL runtime version, source signature/key,
compiler and Python license copy. Per-library license entries remain marked
for native review. This receipt is unsigned and records
`local_source_build_unadmitted`; it does not authorize redistribution or a
product installer. Setup makes the dedicated prefix read only after writing
the receipt so later imports do not add bytecode caches to the exact file set.

Offline byte recheck (no download, compilation or compiler requirement):

```sh
python3 examples/transcribe/dependencies/setup_python_source.py \
  --verify-existing --prefix "$HOME/.local/bashy/python-3.12.15"
```

For an **explicit local** source adapter run, set
`BASHY_TRANSCRIBE_PYTHON_SOURCE_PREFIX` to that prefix and
`BASHY_TRANSCRIBE_PYTHON` to its `bin/python3.12`, then invoke
`launch_python.sh` through Bashy with the acquire request. The launcher checks
the interpreter before execution; the adapter rechecks the receipt and complete
byte inventory before yt-dlp. Without the source prefix opt-in, the existing six
CI-observed executable pins continue to apply. A changed system library causes
reverification to fail until the host's build and license review is repeated.

The two Linux native jobs verified the source signature, built on their native
architectures, enumerated compiler/OpenSSL and loaded libraries, and repeated
the complete installed-byte check with OS network access disabled. Outstanding
gates: review each loaded library's license against the recorded package
copyright, prove an operator-host installation and pre-execution integrity
launcher, and decide product distribution policy. This source recipe does not
close macOS, Windows, FFmpeg, or the six-target transcription gates.

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
3. **Dynamic Import Check**: Post-build inspection (`otool -L` on macOS, `ldd` on Linux, `objdump` on Windows) rejects missing inspection tools, unresolved imports and named OpenMP libraries. This name-based check is not a full transitive license audit. Earlier Windows CI binaries imported external Microsoft VC runtime DLLs; the newer static llvm-mingw candidate does not, but its linked-object notices remain under review.
4. **No Side-Effects**: The build recipe performs zero model downloads, zero media inference, zero FFmpeg dependencies, and no container / OCI requirements.
5. **No False Six-Target PASS**: The recipe reports actual host operating system, architecture, compiler version, and build flags. CI jobs run natively on each runner to establish independent evidence.
