# S503.6 operator-host Python install candidate

This is an explicit external prerequisite route for transcription. Bashy does
not download or redistribute Python or Microsoft VC files. The hosted
`actions/setup-python` interpreter identities are test inputs, not an install
channel. A passing pre-execution launcher gate does not establish product
installation or dependency-license admission.

| Native target | Exact input | Host action | Current status |
| --- | --- | --- | --- |
| Linux amd64, arm64 | Python.org 3.12.15 source tarball, detached signature and pinned release key | Run `setup_python_source.py` under an operator-supplied bootstrap Python with an isolated prefix | `local_source_build_unadmitted` |
| macOS amd64, arm64 | Python.org 3.12.10 universal2 `.pkg`, SHA-256 `8373e58da4ea146b3eb1c1f9834f19a319440b6b679b06050b1f9ee3237aa8e4` | Verify SHA-256, Apple Developer ID Installer team `BMM5U3QVKW`, and Gatekeeper assessment; explicitly install to the fixed framework path | Native operator-host install unverified |
| Windows amd64 | Python.org 3.12.10 `python-3.12.10-amd64.exe`, SHA-256 `67b5635e80ea51072b87941312d00ec8927c4db9ba18938f7ad2d27b328b95fb` | Verify SHA-256 and valid Python Software Foundation Authenticode signature; require pinned native System32 VC bytes; explicitly install per user; probe the installed Python's loaded modules before host verification | Native operator-host install unverified |
| Windows arm64 | Python.org 3.12.10 `python-3.12.10-arm64.exe`, SHA-256 `377ac8fd478987940088e879441e702a71b53164d2a1e6f1d51ff77a7e470258` | Same route with ARM64 Python and loaded VC pins | Native operator-host install unverified |

The macOS and Windows installer pins come from the existing
`external-python-runtime.json` and `windows-external-runtime.json` policies.
`operator-python-admission.json` separately records whether a native installer
and its resulting installed interpreter digest have been measured and approved.
All six entries are currently unadmitted. The CI Python executable SHA-256
printed by `--plan` is only a CI reference: a Python.org package can install
different executable bytes. It cannot satisfy the operator-host installed
interpreter pin. The script pins all three policy files by SHA-256.
`prepare_python_host.sh` checks those policy bytes, chooses the native target,
and reports the exact input with `--plan`. The official [Python release
listing](https://www.python.org/downloads/release/python-31210/) and [Python
signature guidance](https://www.python.org/downloads/) describe these signed
artifacts. The [Windows installer options](https://docs.python.org/3.12/using/windows.html)
support a private per-user target and disabling bundled pip, test files,
Tcl/Tk, launcher and PATH changes. The [Microsoft VC documentation](https://learn.microsoft.com/en-us/cpp/windows/redistributing-visual-cpp-files)
describes the separately installed runtime and its native architecture. Its
latest-download URLs are mutable, so the script does not fetch or install VC.

The research-only [four-target artifact workflow](../../.github/workflows/s503-python-installer-artifacts.yml)
checks both macOS architectures and both Windows architectures on native GitHub-hosted
runners. It downloads the exact URL printed by `--plan`, independently compares
the policy SHA-256, then runs `--verify-artifact`. Each job retains the native
signature and Windows System32 VC observation, the verifier's stdout/stderr,
the staged-copy hash, the Bashy and policy hashes, and a result JSON. Installer
bytes are deleted after the check and are not uploaded. A pass establishes only
the artifact gate; `operator-python-admission.json` remains unadmitted.

The separate [installed-interpreter research workflow](../../.github/workflows/s503-python-installed-native.yml)
runs when its own workflow, helper, verifier or pinned host-prep inputs change on
`s503-contract-integration`; it also supports manual dispatch after GitHub
recognizes the workflow. On disposable GitHub-hosted native macOS amd64/arm64
and Windows amd64/arm64 runners, it repeats the pinned download and the same
`prepare_python_host.sh --verify-artifact` gate, then invokes the official
installer directly. This deliberately does not use the guarded product
`--install` mode. The installer input must equal the reviewed URL, SHA-256,
policy file hashes and [artifact run 40213869256](evidence/python-installer-40213869256/review.json).

Each job retains the native runner identity; installer signature and hash;
literal install argv, exit code and logs; installed interpreter path, hash,
version and process architecture; zlib, bz2, lzma and SSL module origins and
hashes; codec round trips and OpenSSL version; installed license and notice
file hashes with bounded byte copies; and before/after side-effect paths.
macOS also retains file lists for every 3.12 Python package receipt. Windows records
native System32 VC DLLs before and after installation, app-local VC DLLs,
loaded DLL origins, the `py` launcher search path and Python registry paths.
GitHub macOS images may already contain an official Python 3.12 framework.
On a confirmed GitHub-hosted disposable runner only, this research job records
its interpreter hash and every 3.12 `pkgutil --pkg-info` receipt before using
the same official package to overwrite it. It records the receipts again after
installation, requires the framework receipt's install time to change, and
labels that outcome as a reinstall. A fresh path is labeled separately.
The [independent installed-Python review](evidence/python-installed-40214773709/review.json)
observed both Windows architectures load `MSVCP140.dll` from System32 and
`VCRUNTIME140.dll` from the Python install directory. The latter's SHA-256 is
`052ad6a20d375957e82aa6a3c441ea548d89be0981516ca7eb306e063d5027f4`
on amd64 and `560cd0ab1ec81b8b2197b119e198453a9c23d2f9e846f15c79a26bf5d75b68c4`
on arm64. Those are distinct from the System32 `VCRUNTIME140.dll` bytes.
`prepare_python_host.sh` pins this exact reviewed policy and a small probe
script. After the admission policy and installed `python.exe` hash pass, it
checks the app-local `VCRUNTIME140.dll` hash before starting Python. Only then
does the probe run that selected interpreter in isolated mode to observe its
actual loaded module paths and hash their files. The host is reported verified only
if both paths and hashes match the reviewed System32 and app-local identities.
The probe is the only Python code the host-preparation command runs before
that result; it is not a transcription or product-admission result.
The downloaded installer is deleted before artifact upload. Installed files
remain only until the disposable hosted runner is destroyed. A missing
identified macOS framework, failed command, wrong architecture or absent stdlib
module fails the research job. Passing this job supplies evidence for review;
it does not change any operator admission status or authorize redistribution.

## Host procedure

1. Claim the native host under the sprint's resource rules. Record the actual
   OS and architecture, Bashy commit/binary SHA-256, and absence or identity of
   any Python 3.12 installation before changing it. Do not use Rosetta or an
   emulated PowerShell process for a native result.
2. Run `bashy examples/transcribe/dependencies/prepare_python_host.sh --plan`.
   On macOS or Windows, obtain the exact official installer URL it prints and
   retain the downloaded bytes. Run `--verify-artifact PATH`; a changed digest,
   missing/invalid native signature, or wrong Windows VC prerequisite stops
   before installation. Do not substitute a CI tool-cache archive.
3. On an isolated native evidence host, independently measure the exact
   Python.org installer and the interpreter it installs. Review its signature,
   installed files, native dependency origins, notices, and uninstall behavior.
   Record the measured installed-interpreter SHA-256 for that OS and architecture
   in a reviewed operator admission policy; approve the installer independently.
   Until that policy and its script pin are updated, `--install` and
   `--verify-existing` reject every target before native execution.
4. After admission, on a fresh macOS host run `sudo bashy
   .../prepare_python_host.sh --install PATH`; the script refuses an existing
   Python 3.12 framework. On Windows, run `bashy
   .../prepare_python_host.sh --install PATH` as the operator. It installs under
   `%LOCALAPPDATA%\Bashy\Python-3.12.10-{AMD64,ARM64}` with pinned options and
   refuses a preexisting target. Retain the native installer log and exit code.
5. Run `--verify-existing ABSOLUTE_PYTHON_PATH` after installation and again
   with network disabled. This checks the exact interpreter SHA-256 before
   starting it, then checks its Python patch version. Windows also rechecks
   System32 VC versions and hashes plus the app-local `VCRUNTIME140.dll` hash
   before the isolated loaded-module probe,
   and requires the probe's actual System32 `MSVCP140.dll` and app-local
   `VCRUNTIME140.dll` paths and hashes before reporting the host verified.
   Record the exact path, digest, native runtime dependencies and license notices. The actual
   launcher and YouTube adapter still require their own native and offline
   gates.

For Linux, the plan command deliberately does not install a CI tarball. Use
the [signed source setup](../../examples/transcribe/dependencies/README.md)
with the pinned archive, detached signature and release key, a new work
directory, and a new private prefix for research only. Retain
`source-build-receipt.json`, its complete file and dynamic-library inventories,
native build log, module/codec
probe, and an OS-network-denied interpreter check. Set
`BASHY_TRANSCRIBE_PYTHON_SOURCE_PREFIX` only for this source-built interpreter
and use the Bashy pre-execution launcher for acquisition.

## Evidence needed before product admission

- Repeat the operator-host install on Linux, macOS and Windows amd64 and arm64.
  Record exact artifact hashes, signature verdicts, native architecture,
  installer logs, installed interpreter path/hash/version, dependency closure,
  offline byte recheck and a clean uninstall/reinstall or isolated-host reset.
- Windows `MSVCP140.dll` and `VCRUNTIME140.dll` must match the reviewed native
  System32 policy, and the loaded modules must match their separate reviewed
  owners and hashes. An older or updated runtime is a blocker until Microsoft
  package provenance, its license, native files, and a revised pin are
  reviewed. Inspect actual loaded DLL origins and any Python-install-local VC
  DLLs; a System32 hash alone does not prove which copy the loader uses.
- The Linux source build stays `local_source_build_unadmitted`. The
  [native library review](evidence/linux-python-library-license-review.md)
  records host-provided LGPL glibc/libcrypt and other exact Ubuntu libraries,
  plus the Python PSF license. A later host's package notices and any files
  copied into a distributed product need their own review. The known
  Python-build-standalone `_dbm`/Sleepycat route is not substituted here.
- Mac and Windows official installer contents, including any bundled pip,
  extension modules, OpenSSL or app-local Microsoft runtimes, need a complete
  artifact and notice inventory. A verified external install is not permission
  for Bashy to redistribute those files.

No six-target product installer or signed Bashy release is admitted by this
candidate. The official package command and post-install verification are
guarded until their native results and license review are approved.
