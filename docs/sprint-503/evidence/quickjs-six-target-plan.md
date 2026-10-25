# QuickJS-NG six-target retained-byte research gate

The [six-target workflow](../../../.github/workflows/s503-quickjs-six-target.yml)
extends the independently reviewed [two-target pilot](quickjs-native-40220845613-review.md).
The pilot ran pinned QuickJS-NG `v0.17.0` EJS successfully on Linux amd64 and
Windows ARM64, but retained only reported hashes of `qjs` and produced objects.
Independent review could not rehash the executable, examine its machine header,
or assign its actual linked inputs. This workflow retains the bytes for those
checks on Linux, macOS and Windows, amd64 and arm64.

Each native job downloads the exact pinned QuickJS source commit archive and
yt-dlp 2026.10.21 zipapp, verifies their sizes and SHA-256 values, checks
QuickJS license and CMake recipe hashes, builds only `qjs` with shared-library
and mimalloc options off, records the linker map and direct imports, then runs
the bundled EJS 0.8.0 prepared-solver `sig` and `n` challenge. It retains the
source archive, exact executable, QuickJS-target `.o`, `.obj`, `.a` and `.lib`,
their checked copy hashes and modes, source notice candidates, map/import logs,
and `result.json`. CMake compiler probes are excluded. External SDK, CRT and
system libraries are **not uploaded**; their exact observed link/search paths,
on-runner hashes where a file exists, provisional providers and pending notice
status are recorded in `result.json`. Dynamic-cache imports without an on-disk
file remain explicitly unhashed. A 128 MiB per-job copied-byte cap fails closed.
The source
archive allows a reviewer to rehash each notice against the pinned tree. The
actual linked or loaded inputs still require classification from the native
maps/imports and runtime providers; a produced-object list is not that closure.

The six job labels are `ubuntu-24.04`, `ubuntu-24.04-arm`, `macos-15-intel`,
`macos-15`, `windows-2025`, and `windows-11-vs2026-arm`. Native runner identity
and ELF, PE or Mach-O machine header must match the declared target. Every job
has a **30 minute timeout**: at most 180 elapsed runner minutes over six jobs
before any hosted billing multiplier, plus artifact storage/transfer (up to
768 MiB copied bytes before logs). The exact billed minutes depend on the
hosted runner pricing and account plan. Artifacts expire after 14 days, so an
independent reviewer must download and hash the original artifact ZIPs before
then. The four previously untested QuickJS targets may fail at CMake, native
linking or import inspection; such a failure remains explicit evidence.

`research_result: PASS` means only that the pinned source built and controlled
EJS ran on that particular host. The script always sets `product_admitted:
false` and `native_link_notice_closure: BLOCKED`. A product decision needs an
independent executable and archive rehash, per-member and system-runtime
license/notice classification, source and build provenance, safe yt-dlp
temporary-script review, installer/release proof, and real operator media and
six-target transcript gates. This workflow never packages a product runtime,
acquires a YouTube video, or performs speech inference.

The branch trigger is restricted to `s503-contract-integration` changes to the
workflow, script or its guard tests. `workflow_dispatch` also requires the
workflow on the repository default branch. A commit prepared in an isolated
worktree does not start CI; the manager must review and choose whether to push.
