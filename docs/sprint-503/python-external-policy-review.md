# S503.6 Python.org external dependency policy review

Reviewed at Bashy `8fc4dd34` on 2026-10-25. This is an evidence and policy
boundary for the transcription prerequisite, not product or operator-host
admission. All six entries in
[`operator-python-admission.json`](../../examples/transcribe/dependencies/operator-python-admission.json)
remain `unadmitted`; `--install` and `--verify-existing` therefore reject them.

## Exact candidate artifacts

The [Python 3.12.10 release](https://www.python.org/downloads/release/python-31210/)
lists one macOS universal2 installer for Intel and Apple Silicon, a recommended
Windows 64-bit installer, and an experimental Windows ARM64 installer. The
following SHA-256 values are the repository's native, signature-checked
[artifact review](evidence/python-installer-40216443981/review.json) from
[run 40216443981](https://github.com/qiangli/bashy/actions/runs/40216443981).
That run verified staged copies; it did not install or admit them.

| Target | Official Python.org file | SHA-256 | Reviewed installed `python` SHA-256 |
| --- | --- | --- | --- |
| macOS amd64 and arm64 | [`python-3.12.10-macos11.pkg`](https://www.python.org/ftp/python/3.12.10/python-3.12.10-macos11.pkg) | `8373e58da4ea146b3eb1c1f9834f19a319440b6b679b06050b1f9ee3237aa8e4` | `d4f152f2a753c94e0e7935c8ebbe6b2609979e1df7898422b577d0076383d08b` |
| Windows amd64 | [`python-3.12.10-amd64.exe`](https://www.python.org/ftp/python/3.12.10/python-3.12.10-amd64.exe) | `67b5635e80ea51072b87941312d00ec8927c4db9ba18938f7ad2d27b328b95fb` | `4d6f5f81a4bca11191c4c7c6b43632694d0a4ce74e068619d8fdc161d469859a` |
| Windows arm64 | [`python-3.12.10-arm64.exe`](https://www.python.org/ftp/python/3.12.10/python-3.12.10-arm64.exe) | `377ac8fd478987940088e879441e702a71b53164d2a1e6f1d51ff77a7e470258` | `7f1ebbbb04164caf8f8073aafa7f82321fb44695e10fe6a9ebaf2138b63892c4` |

The executable digests are reviewed CI identities, not yet approved
operator-host pins. The prior [installed-interpreter review](evidence/python-installed-40215802421/review.json)
records macOS reinstall over a preexisting CI framework, not a fresh-host
installation. Current-policy installed research
[run 40216950282](https://github.com/qiangli/bashy/actions/runs/40216950282)
was in progress when this note was prepared; inspect its retained artifacts
before drawing a new conclusion. The [host plan](python-operator-install-plan.md)
keeps Linux on a separate, unadmitted Python.org 3.12.15 source-build route.

## Distribution and host boundary

Python's [macOS installation guide](https://docs.python.org/3.12/using/mac.html)
says the universal2 package writes `/Library/Frameworks/Python.framework`, a
`Python 3.12` Applications folder, a `/usr/local/bin` symlink, and shell PATH
changes. A passing reinstall on a preloaded runner does not prove clean
installation, coexistence, or uninstall on an operator host. Require a fresh
native Intel and Apple Silicon host receipt, exact framework executable and
module hashes, package file lists, side-effect inventory, and clean rollback.
Do not remove Apple's `/System/Library/Frameworks/Python.framework`.

The [Python license](https://docs.python.org/3.12/license.html) grants PSF-2.0
rights and warns that incorporated software has other licenses; the Windows
and macOS installers may include OpenSSL. The [Windows installer options](https://docs.python.org/3.12/using/windows.html)
allow the current private per-user `TargetDir` and disabling pip, tests,
Tcl/Tk, launcher, and PATH changes. The exact installed files, extensions,
OpenSSL, notices, and installer side effects still require artifact-specific
review. Installer verification alone is not a complete license inventory.

[Microsoft's VC redistribution rules](https://learn.microsoft.com/en-us/cpp/windows/redistributing-visual-cpp-files)
limit distribution of the redistributable package and individual binaries to
licensed Visual Studio users subject to Microsoft Software License Terms.
[Microsoft recommends](https://learn.microsoft.com/en-us/cpp/windows/redistributing-visual-cpp-files)
central installation as a prerequisite so its libraries receive updates. Its
[latest x64 and ARM64 downloads](https://learn.microsoft.com/en-us/cpp/windows/latest-supported-vc-redist)
are mutable, architecture-specific package URLs, not immutable artifact pins.
No Bashy release should contain a Microsoft VC package or DLL under the current
permissive-only packaging rule. An operator-supplied, separately installed
native VC runtime is the present research route; a Bashy-managed fetch or
redistribution route needs a separate license and artifact decision.

The [Windows policy](../../examples/transcribe/dependencies/windows-external-runtime.json)
pins System32 `MSVCP140.dll` and `VCRUNTIME140.dll` version `14.51.36247.0` and
their architecture-specific hashes. The installed-Python reviews observed
`MSVCP140.dll` resolved from System32 and `VCRUNTIME140.dll` from the Python
install directory, with distinct hashes. The current
[`probe_python_loaded_vc.py`](../../examples/transcribe/dependencies/probe_python_loaded_vc.py)
explicitly loads each DLL by name before checking its resolved path. Its result
proves resolution in that probe, not that Python or yt-dlp imported both DLLs
before the probe. Preserve both System32 and app-local checks; an older or
newer host VC version fails the exact policy until separately reviewed.

| Windows target | System32 `MSVCP140.dll` | System32 `VCRUNTIME140.dll` | Python-local `VCRUNTIME140.dll` |
| --- | --- | --- | --- |
| amd64 | `7c26614e1d733892c2deac7e245ce115504b1d80592dd0a01b08e3e5a55f89ca` | `d1f4225df2cd877dbf130d5668a021dce3f94118455ff5ec952061c30afc9ce7` | `052ad6a20d375957e82aa6a3c441ea548d89be0981516ca7eb306e063d5027f4` |
| arm64 | `167ceac85c2d726c4cd9e39b8881fafdc6de1520c0e67c4f6b271235f9d2a6c5` | `3c56f4167e2b3d8e6338497731e6aae8cd7ec46bd6789f9423a8d9cf9a630310` | `560cd0ab1ec81b8b2197b119e198453a9c23d2f9e846f15c79a26bf5d75b68c4` |

These are runner-observed file identities, not an identified, hash-pinned
Microsoft redistributable installer. Package provenance, license terms, and a
clean operator-host install remain open.

## Decision and admission sequence

1. Keep the external-prerequisite route: the operator obtains and installs
   official Python.org bytes and a native Microsoft VC prerequisite. Bashy
   verifies exact installer digest and signature, then installed files and
   resolved runtime paths. Do not switch to the CI tool cache or silently
   fetch a mutable VC installer.
2. Decide whether the operator-facing product should offer a guarded installer
   after clean native host results. The macOS shared-framework side effects,
   Windows app-local VC bytes, notices, and uninstall path must be reviewed
   before changing any admission flag.
3. Decide whether exact System32 VC hashes remain the product rule, or whether
   signed Microsoft provenance plus an architecture/version floor and recorded
   updates should replace them. The latter needs a new policy, implementation,
   and native evidence; a moving Microsoft URL cannot serve as a pin.
4. Review the current-policy installed run, then repeat on claimed operator
   hosts for all four installer targets. Retain installer and installed
   executable hashes, signature verdicts, actual native process architecture,
   complete installed notices, module and DLL origins, offline recheck, and
   uninstall/reset receipts. Admit one target only after its direct evidence
   and license review are complete. Linux's separate source-build path also
   remains unadmitted.

`product_admitted=false` and `operator_admitted=false` throughout this review.
